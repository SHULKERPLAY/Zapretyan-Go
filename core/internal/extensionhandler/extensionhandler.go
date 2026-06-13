package extensionhandler

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/utils"
)

type ExtensionState struct {
	Name string // Plugin name
	Path string // Path to plugin executable
	Mode string // Plugin execution mode

	// Raw extension config
	Config map[string]interface{}

	State  *exec.Cmd      // Child Process runtime
	Stdout io.ReadCloser  // STDOUT pipe of child process
	Stdin  io.WriteCloser // STDIN pipe of child process
	Stderr io.ReadCloser  // STDERR pipe of child process
}

// Commands to kill or to send plugin configuration.
// Has twin for events in eventor.go.
type CmdMessage struct {
	Ver  int                    `json:"ver"`           // Version of JSON message payload defined in main.go
	Type string                 `json:"type"`          // "rkn" event type
	Kill bool                   `json:"kill"`          // In the "rkn" events must be false
	Path string                 `json:"path"`          // Absolute path to Data directory
	Cfg  map[string]interface{} `json:"cfg,omitempty"` // Using map for plugin config flexibility
}

// Registry of validated Extensions
var ValidExtensions []*ExtensionState

// superviseStream handles plugin and restarting it if fallen
func superviseStream(ctx context.Context, wg *sync.WaitGroup, ext *ExtensionState) {
	// WaitGroup -1 when exited from for{} loop
	defer wg.Done()
	defer slog.Debug("superviseStream() ended", "extension", ext.Name)

	// Start attempts counter
	var startRetries int 

	// Return from loop only if core context was closed
	for {
		// Increment start attempts counter
		startRetries++

		// If extension was killed by end of context - Not restarting it
		if ctx.Err() != nil {
			return
		}

		// If extension restarting 10 times then we need to disable it
		if startRetries > 10 {
			slog.Error("To many crashes of extension", "name", ext.Name)
			DisableExtension(ext.Name)
			return
		}
		
		slog.Info("Starting extension", "name", ext.Name, "try", startRetries)

		// Define child process
		cmd := utils.ExecuteOS(ext.Path)

		// Put process state in struct
		ext.State = cmd

		// in and out streams to extension struct
		// Catch data from stdout
		ext.Stdout, _ = cmd.StdoutPipe()
		// Catch logs from stderr
		ext.Stderr, _ = cmd.StderrPipe()
		// Init stdin
		ext.Stdin, _ = cmd.StdinPipe()

		// Start child process
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
		json.NewEncoder(ext.Stdin).Encode(CmdMessage{
			Ver:  config.Params.JsonVer,
			Type: "cmd",
			Kill: false,
			Path: config.DataParams.DataDirectory,
			Cfg:  ext.Config,
		})

		// Waiting for process end
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		// Core shutdown (Interrupt closing core context)
		case <-ctx.Done():
			slog.Info("Sent shutdown message to extension...", "name", ext.Name)
			json.NewEncoder(ext.Stdin).Encode(CmdMessage{
				Ver:  config.Params.JsonVer,
				Type: "cmd",
				Kill: true,
				Path: config.DataParams.DataDirectory,
				Cfg:  make(map[string]interface{}),
			})
			// Wait for process to close itself for 5s else send SIGKILL
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

// Start all extensions with type `STREAM` before first scan
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
	if ctx.Err() != nil {
		slog.Info("All extensions stopped.")
	} else {
		slog.Error("All STREAM extensions has stopped!")
	}
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
	localCtx, cancel := context.WithTimeout(ctx, time.Duration(config.Params.ExtOnceCtxTimeout-10)*time.Second)
	defer cancel() // Clean resources

	ext.State = utils.ExecuteOS(ext.Path)

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
	go func(cancel context.CancelFunc) {
		// Channels to get results of json parse
		dataCh := make(chan any, 1)
		errCh := make(chan error, 1)

		var void any

		if err := json.NewDecoder(ext.Stdout).Decode(&void); err != nil {
			errCh <- err
		} else {
			dataCh <- void
		}

		// Waiting for first data with 10s timeout
		select {
		case <-dataCh:
			slog.Debug("Got first data in stdout", "name", ext.Name)

			// Send configuration on extension start
			slog.Info("Sent config to extension", "name", ext.Name)
			json.NewEncoder(ext.Stdin).Encode(CmdMessage{
				Ver:  config.Params.JsonVer,
				Type: "cmd",
				Kill: false,
				Path: config.DataParams.DataDirectory,
				Cfg:  ext.Config,
			})

			startedChan <- struct{}{}
		case err := <-errCh:
			slog.Warn("Extension output stream ended or sent invalid JSON", "name", ext.Name, "err", err)
			close(startedChan)
			cancel()
			return

		case <-time.After(10 * time.Second):
			slog.Warn("Extension did not responded in time", "name", ext.Name)
			close(startedChan)
			cancel()
			return
		}

		io.Copy(io.Discard, ext.Stdout) // Purge all further data from buffer (As it not needed anymore)
	}(cancel)

	// Waiting for process end
	done := make(chan error, 1)
	go func() { done <- ext.State.Wait() }()

	select {
	// Core shutdown (Interrupt closing core context)
	case <-localCtx.Done():
		slog.Info("Sent shutdown message to extension...", "name", ext.Name)
		json.NewEncoder(ext.Stdin).Encode(CmdMessage{
			Ver:  config.Params.JsonVer,
			Type: "cmd",
			Kill: true,
			Path: config.DataParams.DataDirectory,
			Cfg:  make(map[string]interface{}),
		})

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

// Removes extension by its name from ValidExtensions array preventing it from starting.
// Replaces requested extension with last from array and removing last index
func DisableExtension(name string) {
	for i, ext := range ValidExtensions {
		if ext.Name == name {
			// Replace removing element with last object of slice
			ValidExtensions[i] = ValidExtensions[len(ValidExtensions)-1]

			// Clear last object to delete it by GC
			ValidExtensions[len(ValidExtensions)-1] = nil

			// Cut last (already empty) index of slice
			ValidExtensions = ValidExtensions[:len(ValidExtensions)-1]

			slog.Error("Extension was removed from core until next start", "name", name)
			slog.Info("Current started", "extensions", GetExtensionsListString())
			return // Return as we need to remove only one matching element
		}
	}
}

// Return string with names of all started extensions separated by ", "
func GetExtensionsListString() string {
	// Return if no extensions started
	if len(ValidExtensions) < 1 {
		return "No any extensions started"
	}

	// Create slice of strings with preallocated memory depend on extension count
	names := make([]string, len(ValidExtensions))

	// Collect Names in created array
	for i, ext := range ValidExtensions {
		names[i] = ext.Name
	}

	// Merge array in one string with separator
	return strings.Join(names, ", ")
}
