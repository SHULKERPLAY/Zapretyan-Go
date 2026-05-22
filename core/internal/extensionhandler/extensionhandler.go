package extensionhandler

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
	"zapretyan-go/internal/config"
)

type ExtensionState struct {
	Name string
	Path string
	Mode string

	// Extension config
	Config map[string]interface{}

	// Runtime
	State  *exec.Cmd
	Stdout io.ReadCloser
	Stdin  io.WriteCloser
	Stderr io.ReadCloser
}

// System commands for plugin (example kill = true)
type CmdMessage struct {
	Ver  int                    `json:"ver"`
	Type string                 `json:"type"`
	Kill bool                   `json:"kill"`
	Cfg  map[string]interface{} `json:"cfg"`
}

// Registry of validated Extensions
var ValidExtensions []*ExtensionState

// superviseStream handles plugin and restarting it if fallen
func superviseStream(ctx context.Context, wg *sync.WaitGroup, ext *ExtensionState) {
	// WaitGroup -1 when exited from for{} loop
	defer wg.Done()
	defer slog.Debug("superviseStream() ended", "extension", ext.Name)

	// Return from loop only if core context was closed
	for {
		// If extension was killed by end of context - Not restarting it
		if ctx.Err() != nil {
			return
		}

		cmd := exec.Command(ext.Path)

		// Put process state in struct
		ext.State = cmd

		// in and out streams to extension struct
		// Catch data from stdout
		ext.Stdout, _ = cmd.StdoutPipe()
		// Catch logs from stderr
		ext.Stderr, _ = cmd.StderrPipe()
		// Init stdin
		ext.Stdin, _ = cmd.StdinPipe()

		if err := cmd.Start(); err != nil {
			slog.Error("Failed to start STREAM extension", "name", ext.Name, "err", err)
			time.Sleep(5 * time.Second) // Restart delay
			continue
		}

		slog.Info("STREAM extension started", "name", ext.Name)

		// Output Log from stderr
		go func() {
			scanner := bufio.NewScanner(ext.Stderr)
			for scanner.Scan() {
				// Formatting plugin output
				slog.Info("PLUGIN:", "name", ext.Name, "msg", scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				slog.Error("Log scanner error", "err", err)
			}
		}()

		// goroutine for processing stdout
		// This data lives here and not displaying in console
		go func() {
			// We can add read logic if needed (example: json.NewDecoder)
			io.Copy(io.Discard, ext.Stdout) // Purge all further data from buffer (As it not needed anymore)
		}()

		// Send configuration on extension start
		slog.Info("Sent config to extension", "name", ext.Name)
		json.NewEncoder(ext.Stdin).Encode(CmdMessage{Ver: config.Params.JsonVer, Type: "cmd", Kill: false, Cfg: ext.Config})

		// Waiting for process end
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		// Core shutdown (Interrupt closing core context)
		case <-ctx.Done():
			slog.Info("Sent shutdown message to extension...", "name", ext.Name)
			json.NewEncoder(ext.Stdin).Encode(CmdMessage{Ver: config.Params.JsonVer, Type: "cmd", Kill: true, Cfg: make(map[string]interface{})})

			select {
			case <-done:
				slog.Info("Extension successfuly stopped", "name", ext.Name)
			case <-time.After(5 * time.Second):
				slog.Warn("Extension process did not stop in time. Executing Process.Kill()!", "name", ext.Name)
				cmd.Process.Kill()
			}
			return

		// If context is alive but extension process is dead
		case err := <-done:
			slog.Warn("STREAM extension has crashed. Restarting...", "name", ext.Name, "err", err)
			time.Sleep(5 * time.Second) // Restart Delay
		}
		// Loop restart if select is ended
	}
}

func StartSteamExtensions(ctx context.Context, globalWg *sync.WaitGroup) {
	defer globalWg.Done() // When all local workgroups has stopped we send wg.Done() to top function
	defer slog.Debug("StartSteamExtensions() ended")

	// Create localWaitgroup for handling started extensions
	var localWg sync.WaitGroup

	streamCnt := 0

	// Starting STREAM extensions
	for _, ext := range ValidExtensions {
		if ext.Mode == "STREAM" {
			streamCnt++
			localWg.Add(1)
			go superviseStream(ctx, &localWg, ext)
		}
	}

	time.Sleep(5 * time.Second)   // Giving extensions some time for start their internal processes
	config.Params.ExtReady = true // Extensions loaded

	if streamCnt < 1 {
		slog.Info("No STREAM extensions found")
		return // Stop function if no extensions to start
	}

	// Wait until all plugins stop
	localWg.Wait()
	slog.Info("All extensions stopped.")
}

// RunOnceExtension starts extension with mode ONCE and force kills it if timeout is reached.
// Channel startedChan sends a signal to executor that process is started successfuly.
func RunOnceExtension(ctx context.Context, wg *sync.WaitGroup, ext *ExtensionState, startedChan chan<- struct{}) {
	defer slog.Debug("RunOnceExtension() ended")
	defer wg.Done() // Report that function is ended

	if ext.Mode != "ONCE" {
		slog.Error("EXTENSION MODE NOT EQUALS 'ONCE'", "name", ext.Name)
		close(startedChan)
		return
	}

	// Create context that cancel by timer or by global context end
	localCtx, cancel := context.WithTimeout(ctx, time.Duration(config.Params.ExtOnceCtxTimeout - 10) * time.Second)
	defer cancel() // Clean resources

	ext.State = exec.Command(ext.Path)

	// in and out streams to extension struct
	// Catch data from stdout
	ext.Stdout, _ = ext.State.StdoutPipe()
	// Catch logs from stderr
	ext.Stderr, _ = ext.State.StderrPipe()
	// Init stdin
	ext.Stdin, _ = ext.State.StdinPipe()

	// Start process
	if err := ext.State.Start(); err != nil {
		slog.Error("Failed to start extension", "name", ext.Name, "err", err)
		close(startedChan)
		return
	}

	// Output Log from stderr
	go func() {
		scanner := bufio.NewScanner(ext.Stderr)
		for scanner.Scan() {
			// Formatting plugin output
			slog.Info("PLUGIN:", "name", ext.Name, "msg", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			slog.Error("Log scanner error", "err", err)
		}
	}()

	// goroutine for processing stdout
	// Sent signal to executor when first data is arrived
	go func() {
		var void map[string]interface{}
		if err := json.NewDecoder(ext.Stdout).Decode(&void); err != nil {
			slog.Warn("Extension output stream ended or sent invalid JSON", "name", ext.Name, "err", err)
			close(startedChan)
			return
		}
		slog.Debug("Got first data in stdout", "name", ext.Name)
		startedChan <- struct{}{}
		io.Copy(io.Discard, ext.Stdout) // Purge all further data from buffer (As it not needed anymore)
	}()

	// Waiting for process end
	done := make(chan error, 1)
	go func() { done <- ext.State.Wait() }()

	select {
	// Core shutdown (Interrupt closing core context)
	case <-localCtx.Done():
		slog.Info("Sent shutdown message to extension...", "name", ext.Name)
		json.NewEncoder(ext.Stdin).Encode(CmdMessage{Ver: config.Params.JsonVer, Type: "cmd", Kill: true, Cfg: make(map[string]interface{})})

		select {
		case <-done:
			slog.Info("Extension successfuly stopped", "name", ext.Name)
		case <-time.After(5 * time.Second):
			slog.Warn("Extension process did not stop in time. Executing Process.Kill()!", "name", ext.Name)
			ext.State.Process.Kill()
		}
		return

	// If context is alive but extension process is dead
	case <-done:
		slog.Info("Extension successfuly closed", "name", ext.Name)
	}
}
