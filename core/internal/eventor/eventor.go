package eventor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/diffprocess"
	"zapretyan-go/internal/extensionhandler"
)

// EventMessage - STDIN extensions message format.
// Has twin for commands in extensionhandler.go.
type EventMessage struct {
	Ver  int                    `json:"ver"`           // Version of JSON message payload defined in main.go
	Type string                 `json:"type"`          // "rkn" event type
	Kill bool                   `json:"kill"`          // In the "rkn" events must be false
	Path string                 `json:"path"`          // Absolute path to Data directory
	Cfg  map[string]interface{} `json:"cfg,omitempty"` // Using map for plugin config flexibility
	Diff DiffData               `json:"diff"`
}

// DiffData contains structured changelogs
type DiffData struct {
	Banned     DiffGroup `json:"banned"`      // Domains added to list
	Unbanned   DiffGroup `json:"unbanned"`    // Domains removed from list
	BannedIP   DiffGroup `json:"banned_ip"`   // IPs added to list
	UnbannedIP DiffGroup `json:"unbanned_ip"` // IPs removed from list
}

// DiffGroup represents structure of each change
type DiffGroup struct {
	Empty  bool     `json:"empty"`
	Length int      `json:"length"`
	Data   []string `json:"data"`
	Total  int      `json:"total,omitempty"` // omitempty hides this field in unbanned
}

// Create new event from diff. Requires filepaths to new domain and IPs lists
func CreateRknEvent(ctx context.Context, rawDiff diffprocess.RawDiff, dpath, ipath string) {
	defer slog.Debug("CreateRknEvent() ended")
	
	// Check if any events is started
	if len(extensionhandler.ValidExtensions) < 1 {
		slog.Warn("No extensions started. Skip event creation")
		return
	}

	// Structure changes data
	banned := fillRknDiffGroup(rawDiff.Domain.Added, true, dpath)
	unbanned := fillRknDiffGroup(rawDiff.Domain.Removed, false, dpath)
	bannedIp := fillRknDiffGroup(rawDiff.Ip.Added, true, ipath)
	unbannedIp := fillRknDiffGroup(rawDiff.Ip.Removed, false, ipath)

	// Check if all lists is empty
	allempty := banned.Empty && unbanned.Empty && bannedIp.Empty && unbannedIp.Empty

	// Compile main event message and send it to each loaded extension
	if allempty && !config.Params.SendEmptyEvent {
		// Return if all diffs empty and we not allowed to send empty diffs
		slog.Info("Diffs empty. Skip event creation")
		return
	} else {
		// Build DiffData object
		data := DiffData{
			Banned:     banned,
			Unbanned:   unbanned,
			BannedIP:   bannedIp,
			UnbannedIP: unbannedIp,
		}

		sendRknEvent(ctx, data)
	}
}

// Build DiffGroup object. If isTotal false - Total (int) stays empty.
// Requires path to file to count length
func fillRknDiffGroup(diff []string, isTotal bool, path string) DiffGroup {
	defer slog.Debug("fillRknDiffGroup() ended")

	// Count array length
	length := len(diff)

	// Is array empty?
	var empty = false
	if length < 1 {
		empty = true
	}

	// Total length of source list
	var total int
	if isTotal {
		tot, err := diffprocess.CountNonEmptyLines(path)
		if err != nil {
			slog.Error("Error counting total length", "file", path, "err", err)
		}
		total = tot
	}

	// Build group
	group := DiffGroup{
		Empty:  empty,
		Length: length,
		Data:   diff,
		Total:  total,
	}

	return group
}

func sendRknEvent(globalCtx context.Context, data DiffData) {
	defer slog.Debug("sendRknEvent() ended")
	slog.Info("Sending event to extensions")

	// WaitGroup for started extensions
	var wg sync.WaitGroup

	// For debug write to not repeat it
	var eventwritten bool

	// List extensions and send event
	for _, ext := range extensionhandler.ValidExtensions {
		event := EventMessage{
			Ver:  config.Params.JsonVer,
			Type: "rkn",
			Kill: false,
			Path: config.DataParams.DataDirectory,
			Cfg:  ext.Config,
			Diff: data,
		}

		if ext.Mode == "ONCE" {
			startedChan := make(chan struct{})
			wg.Add(1)
			go extensionhandler.RunOnceExtension(globalCtx, &wg, ext, startedChan)

			// Wait for start and first data in stdout
			_, ok := <-startedChan
			if !ok {
				slog.Error("Extension skipped", "name", ext.Name)
				continue
			}
		}

		// Encone event in one JSON line with newline (\n) in the end
		slog.Info("Sent new event to", "extension", ext.Name)
		if err := json.NewEncoder(ext.Stdin).Encode(event); err != nil {
			slog.Error("Error while sending event to stdin", "name", ext.Name, "err", err)
		}

		// Write event if flag is set for debugging purposes
		if config.Params.DumpEvent && !eventwritten {
			go saveEventToFile(event)
			eventwritten = true
		}
	}

	wg.Wait() // Wait for all ONCE extensions to stop
}

// saveEventToFile saves event into json file with timestamp
func saveEventToFile(event any) {
	defer slog.Debug("saveEventToFile() ended")
	// Format filename based on current date-time
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("event_%s.json", timestamp)

	// Check if directory exists
	if err := os.MkdirAll(filepath.Join(config.DataParams.DataDirectory, "debug"), 0755); err != nil {
		return
	}

	// Path to save
	filePath := filepath.Join(config.DataParams.DataDirectory, "debug", filename)

	// Open file to write
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		slog.Error("Failed to create event file", "path", filePath, "err", err)
		return
	}
	defer file.Close()

	// Create buffered writer
	bufferedWriter := bufio.NewWriter(file)

	// Write into buffer bypassing heap
	encoder := json.NewEncoder(bufferedWriter)
	if err := encoder.Encode(event); err != nil {
		slog.Error("Failed to encode event to file", "path", filePath, "err", err)
		return
	}

	// Flush buffer from RAM before end
	if err := bufferedWriter.Flush(); err != nil {
		slog.Error("Failed to flush event buffer to disk", "path", filePath, "err", err)
	}
	slog.Info("Event written as", "name", filename)
}
