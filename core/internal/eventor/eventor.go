package eventor

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
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
	Path string					`json:"path"`		   // Absolute path to Data directory
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
func CreateRknEvent(ctx context.Context, wg *sync.WaitGroup, rawDiff diffprocess.RawDiff, dpath, ipath string) {
	defer slog.Debug("CreateRknEvent() ended")
	defer wg.Done() // Report that function is ended

	// Structure changes data
	banned := fillRknDiffGroup(rawDiff.Domain.Added, true, dpath)
	unbanned := fillRknDiffGroup(rawDiff.Domain.Removed, false, dpath)
	bannedIp := fillRknDiffGroup(rawDiff.Ip.Added, true, ipath)
	unbannedIp := fillRknDiffGroup(rawDiff.Ip.Removed, false, ipath)

	// Compile main event message and send it to each loaded extension
	if banned.Empty && unbanned.Empty && bannedIp.Empty && unbannedIp.Empty && config.Params.SendEmptyEvent {
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

	// WaitGroup for started extensions
	var wg sync.WaitGroup

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
	}

	wg.Wait() // Wait for all ONCE extensions to stop
}
