package extensionhandler

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

type ExtensionState struct {
	Name string
	Path string
	Mode string
	State *exec.Cmd
	Stdout io.ReadCloser
	Stdin io.WriteCloser
	Stderr io.ReadCloser
}

type Payload struct {
	Message string `json:"message"`
}

type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
}

func main() {
	allowCustom := flag.Bool("allow-custom-extensions", false, "Allow Custom Extensions")
	flag.Parse()

	if !*allowCustom {
		slog.Error("CUSTOM EXTENSIONS IS NOT ALLOWED!")
	}

	// TODO: Check Mode for every plugin on startup
	// Collect all plugins in one place
	// Add to RPC JSON messages: { type: "rkn"/"cmd", kill: false/true }
	Plugin := ExtensionState{
		Name: "Test Name",
		Path: "./extensions/my_plugin.exe",
	}

	// Process extension
    handlePlugin(&Plugin, Payload{Message: "Тестовые данные"})
}

func handlePlugin(extension *ExtensionState, data Payload) {

	// Instead of putting in private variable we put process state in struct
	extension.State = exec.Command(extension.Path)

	// in and out streams to extension struct
	// Catch data from stdout
	extension.Stdout, _ = extension.State.StdoutPipe()
	// Catch logs from stderr
	extension.Stderr, _ = extension.State.StderrPipe()
	// Init stdin
	extension.Stdin, _ = extension.State.StdinPipe()

	if err := extension.State.Start(); err != nil {
		slog.Error("Error while starting", "plugin", extension.Path, "err", err)
		return
	}

	var response Handshake
	// Read until get full JSON reply
	err := json.NewDecoder(extension.Stdout).Decode(&response)
	if err != nil {
		slog.Error("PLUGIN SENT BAD JSON:", err)
	}

	// goroutine for processing stdout
	// This data lives here and not displaying in console
	go func() {
		// We can add read logic if needed (example: json.NewDecoder)
		io.Copy(io.Discard, extension.Stdout) // Purge all further data from buffer (As it not needed anymore)
	}()

	// Output Log from stderr
	go func() {
		scanner := bufio.NewScanner(extension.Stderr)
		for scanner.Scan() {
			// Formatting plugin output
			slog.Info("PLUGIN:", "name", extension.Name, "msg", scanner.Text())
		}
	}()

	// "ONCE" starts extension process once when report is generated and waiting until it ends
	// "STREAM" starts extension long-term process with the core and sending reports in the stdin of extension
	switch string(response.Mode) {
	case "ONCE":
		slog.Info("Plugin", extension.Name, "mode", response.Mode)
		json.NewEncoder(extension.Stdin).Encode(data)
		extension.Stdin.Close()
		extension.State.Wait()

	case "STREAM":
		slog.Info("Plugin", extension.Name, "mode", response.Mode)
		// Start Goroutine which will be handling channel
		go func() {
			encoder := json.NewEncoder(extension.Stdin)
			for i := 0; i < 5; i++ { // Пример: шлем данные 5 раз
				data.Message = fmt.Sprintf("Пакет №%d", i)
				encoder.Encode(data)
				time.Sleep(1 * time.Second)
			}
			extension.Stdin.Close() // Close when no data left
		}()

		extension.State.Wait()

	default:
		slog.Error("Unknown Plugin Mode", "mode", response.Mode)
		return
	}
}