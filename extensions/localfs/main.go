package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Handshake - structure to first reply to core
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

// PluginConfig - Part of structure that stores in "cfg"
type PluginConfig struct {
	SaveLastEntries int    `json:"save_last_entries"` // Number of entries to save in Directory
	Directory 		string `json:"localfs_dir"` 	  // Directory to store data inside core data dir
}

// BaseEvent - Base structure to understand what delivered without full parse
type BaseEvent struct {
	Ver  int           `json:"ver"`
	Type string        `json:"type"`
	Kill bool          `json:"kill"`
	Path string		   `json:"path"` // Absolute path to Data directory
	Cfg  *PluginConfig `json:"cfg"`  // Field can be empty so use the pointer
}

// Plugin internal specs
const pmode = "ONCE"		  // Can also be "STREAM"
const pjsonver = 1			  // Expected JSON version
const pver = "1.0.0@u11i51pi"

var config    *PluginConfig
var targetDir string		 // Default directory

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
func logMsg(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func main() {
	// Send handshake into stdout on start
	handshake := Handshake{
		Mode:    pmode,
		Version: pver,
		JsonVer: pjsonver,
	}

	if err := json.NewEncoder(os.Stdout).Encode(handshake); err != nil {
		logMsg("FATAL: Error sending Handshake (%v)", err)
		os.Exit(1)
	}

	// Listening Stdin all lifetime
	decoder := json.NewDecoder(os.Stdin)
	
	for {
		// Read raw JSON data to save it without losses
		var rawEvent json.RawMessage
		err := decoder.Decode(&rawEvent)
		if err != nil {
			// If core closed Stdin, closing
			if err == io.EOF {
				logMsg("stdin closed by core. Shutdown.")
				os.Exit(0)
			}
			logMsg("Error reading JSON from stdin: %v", err)
			continue
		}

		handleEvent(rawEvent)
	}
}

// handleEvent decompiles event and forward further
func handleEvent(raw json.RawMessage) {
	var base BaseEvent
	if err := json.Unmarshal(raw, &base); err != nil {
		logMsg("Error parsing base structure: %v", err)
		return
	}

	// Exit if "kill": true
	if base.Kill {
		logMsg("Got KILL signal. Shutdown.")
		os.Exit(0)
	}

	// Check proto ver
	if base.Ver != pjsonver && base.Ver != 0 {
		logMsg("Warning: Event version (%d) does not match (%d)", base.Ver, pjsonver)
	}

	switch base.Type {
		// Usually if core sends "cmd" type with "kill":false then core just sent config data to plugin
	case "cmd":
		// If config is still empty, and JSON has "cfg" then initialize config
		if config == nil && base.Cfg != nil {
			config = base.Cfg
			logMsg("Configuration loaded. File limit: %d, subdir: ./%v", config.SaveLastEntries, config.Directory)
		}

		// Load Directory
		if targetDir == "" && base.Path != "" && config.Directory != "" {
			targetDir = filepath.Join(base.Path, config.Directory)
			logMsg("Directory: %v", targetDir)
		}

	case "rkn":
		saveLimit := 100 // Safe default if config still not loaded
		if config != nil && config.SaveLastEntries > 0 {
			saveLimit = config.SaveLastEntries
		}
		processRknEvent(raw, saveLimit)

	default:
		logMsg("Unknown event type: %s", base.Type)
	}
}

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
func processRknEvent(raw json.RawMessage, saveLimit int) {
	// Format single string JSON as JSON with spaces
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		logMsg("Error formatting JSON: %v", err)
		return
	}

	// Check if directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		logMsg("Error creating directory %s: %v", targetDir, err)
		return
	}

	// Save file with date and time
	fileName := time.Now().Format("2006-01-02_15-04-05") + ".txt"
	filePath := filepath.Join(targetDir, fileName)

	if err := os.WriteFile(filePath, pretty.Bytes(), 0644); err != nil {
		logMsg("Error writing file %s: %v", filePath, err)
		return
	}
	
	logMsg("Event written successfuly: %s", fileName)

	// Start file rotation end exit
	rotateFiles(saveLimit)
}

// rotateFiles Checks limit of files in folder and removes oldest
func rotateFiles(limit int) {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		logMsg("Error reading directory contents for rotation: %v", err)
		return
	}

	// Collect only .txt files to not erase something else
	var files []os.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".txt" {
			info, err := entry.Info()
			if err == nil {
				files = append(files, info)
			}
		}
	}

	// If files less or equal to limit, doing nothing
	if len(files) <= limit {
		return
	}

	// Sort files by modified time (old to new)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	// Count how many files to remove
	filesToDelete := len(files) - limit
	
	// Remove N old files
	for i := 0; i < filesToDelete; i++ {
		path := filepath.Join(targetDir, files[i].Name())
		if err := os.Remove(path); err != nil {
			logMsg("Failed to remove old file %s: %v", path, err)
		} else {
			logMsg("Cleanup: Removed %s", files[i].Name())
		}
	}

	// End of task. If mode set to STREAM - continue listening from core
	if pmode == "ONCE" {
		os.Exit(0)
	}
}