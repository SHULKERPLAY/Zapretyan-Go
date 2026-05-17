package extensionloader

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/extensionhandler"
)

// Handshake is expecting answer from extension on start
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

func validateAndBurn(name string, version string) bool {
	if config.Params.AllowCustom {
		return true
	}

	// Check if known plugin
	expectedTag, exists := config.Params.Registry[name]
	if !exists {
		return false // This plugin already active
	}

	// Check tag
	if strings.Contains(version, expectedTag) {
		// Burn from registry
		delete(config.Params.Registry, name)
		return true
	}

	return false
}

func verifyExtension(rawCfg map[string]interface{}) (*extensionhandler.ExtensionState, error) {
	name, _ := rawCfg["name"].(string)
	rawpath, _ := rawCfg["path"].(string)
	enabled, _ := rawCfg["enabled"].(bool)

	if !enabled {
		return nil, fmt.Errorf("extension disabled in config.toml")
	}

	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return nil, fmt.Errorf("INVALID PLUGIN NAME!")
	}

	cleanPath := strings.TrimSpace(rawpath)
	if cleanPath == "" {
		return nil, fmt.Errorf("Path of plugin %v is not valid!", name)
	}

	// Build OS specific path
	path := filepath.Join(config.Params.AppPath, config.ExecPath(rawpath))

	// Start process to validate
	cmd := exec.Command(path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error while starting: %v", err)
	}

	// Channel to get results of json parse
	hsCh := make(chan Handshake, 1)
	errCh := make(chan error, 1)

	go func() {
		var hs Handshake
		if err := json.NewDecoder(stdout).Decode(&hs); err != nil {
			errCh <- err
		} else {
			hsCh <- hs
		}
	}()

	var mode string

	// Waiting for Handshake with 5s timeout
	select {
	case hs := <-hsCh:
		if !validateAndBurn(name, hs.Version) {
			return nil, fmt.Errorf("Plugin %v already loaded or not valid! To load custom plugins set 'allow_custom_extensions = true' in config.toml", name)
		}
		if hs.JsonVer != config.Params.JsonVer {
			slog.Warn("Plugin JSON version mismatch! Plugin can expirience issues parsing data", "corejsonver", config.Params.JsonVer, "extjsonver", hs.JsonVer)
		}
		mode = hs.Mode
		slog.Info("Handshake completed", "name", name, "mode", mode, "version", hs.Version)
	case err := <-errCh:
		cmd.Process.Kill()
		return nil, fmt.Errorf("error reading Handshake: %v", err)
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		return nil, fmt.Errorf("TIMEOUT: Extension did not responded in time")
	}

	// Send shutdown cmd
	killCmd := extensionhandler.CmdMessage{Ver: 1, Type: "cmd", Kill: true}
	json.NewEncoder(stdin).Encode(killCmd)

	// Chan for waiting process end
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	// Waiting Exit code 0 with 5s timeout
	select {
	case err := <-waitCh:
		if err != nil {
			return nil, fmt.Errorf("An error has occured: %v", err)
		}
		slog.Info("Extension successfuly validated", "name", name)
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		return nil, fmt.Errorf("TIMEOUT: Extension did not performed shutdown in time")
	}

	// Compile valid extension state
	return &extensionhandler.ExtensionState{
		Name:   name,
		Path:   path,
		Mode:   mode,
		Config: rawCfg, // Write plugin config
	}, nil
}

func InitExtensions() {
	// Handshake phase
	slog.Info("Extension Initializing started")
	for _, rawCfg := range config.RawCfg.Extensions {
		name, _ := rawCfg["name"].(string)

		extState, err := verifyExtension(rawCfg)
		if err != nil {
			slog.Warn("SKIP EXTENSION", "name", name, "reason", err)
			continue
		}
		extensionhandler.ValidExtensions = append(extensionhandler.ValidExtensions, extState)
	}

	if len(extensionhandler.ValidExtensions) < 1 {
		slog.Error("FATAL: AT LEAST ONE EXTENSION MUST BE STARTED!")
		os.Exit(1)
	}

	slog.Info("Extension initialize completed", "valid_count", len(extensionhandler.ValidExtensions))
}
