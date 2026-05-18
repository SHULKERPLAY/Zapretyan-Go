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

// STDIN extensions message format
type EventMessage struct {
	Ver  int                    `json:"ver"`
	Type string                 `json:"type"`
	Kill bool                   `json:"kill"`
	Cfg  map[string]interface{} `json:"cfg"`
	Diff interface{}            `json:"diff"`
}

// System commands for plugin (example kill = true)
type CmdMessage struct {
	Ver  int    `json:"ver"`
	Type string `json:"type"`
	Kill bool   `json:"kill"`
}

// Registry of validated Extensions
var ValidExtensions []*ExtensionState

// superviseStream handles plugin and restarting it if fallen
func superviseStream(ctx context.Context, wg *sync.WaitGroup, ext *ExtensionState) {
	defer wg.Done()
	defer slog.Debug("superviseStream() ended", "extension", ext.Name)

	for {
		// If core shutting down - not restarting
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
		}()

		// goroutine for processing stdout
		// This data lives here and not displaying in console
		go func() {
			// We can add read logic if needed (example: json.NewDecoder)
			io.Copy(io.Discard, ext.Stdout) // Purge all further data from buffer (As it not needed anymore)
		}()

		// Waiting for process end
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		// Core shutdown (recieved SIG)
		case <-ctx.Done():
			slog.Info("Sent shutdown message to extension...", "name", ext.Name)
			json.NewEncoder(ext.Stdin).Encode(CmdMessage{Ver: 1, Type: "cmd", Kill: true})

			select {
			case <-done:
				slog.Info("Extension successfuly stopped", "name", ext.Name)
			case <-time.After(5 * time.Second):
				slog.Warn("Extension process did not stop in time. Executing Process.Kill()!", "name", ext.Name)
				cmd.Process.Kill()
			}
			return

		// Extension crashed
		case err := <-done:
			slog.Warn("STREAM extension has crashed. Restarting...", "name", ext.Name, "err", err)
			time.Sleep(5 * time.Second) // Restart Delay
		}
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

	time.Sleep(5 * time.Second) // Giving extensions some time for start their internal processes
	config.Params.ExtReady = true // Extensions loaded

	if streamCnt < 1 {
		slog.Info("No STREAM extensions found")
		return // Stop function if no extensions to start
	}

	// Wait until all plugins stop
	localWg.Wait()
	slog.Info("All extensions stopped.")
}
