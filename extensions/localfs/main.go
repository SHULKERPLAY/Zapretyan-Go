package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// Handshake is a structure of first reply to core
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

// PluginConfig - Part of structure that stores in "cfg"
type PluginConfig struct {
	SaveLastEntries int    `json:"save_last_entries"` // Number of entries to save in Directory
	Directory       string `json:"localfs_dir"`       // Directory to store data inside core data dir
}

// BaseEvent - Base structure to understand what delivered without full parse
type BaseEvent struct {
	Ver  int           `json:"ver"`  // Core JSON format version
	Type string        `json:"type"` // "cmd" for commands and config send, "rkn" for events
	Kill bool          `json:"kill"` // If true process must exit immidiately
	Path string        `json:"path"` // Absolute path to Data directory
	Cfg  *PluginConfig `json:"cfg"`  // Field can be empty so use the pointer
}

// PLUGIN MODES: If your plugin returned "ONCE" it will be closed after validation and opened on every event.
// Time for processing event in mode "ONCE" IS LIMITED by core settings. You must process the event
// within (once_ctx_deadline) seconds end exit or your plugin will recieve kill cmd and SIGKILL if plugin not exited.

// PLUGIN MODES: If your plugin returned "STREAM" it will be started after validation step and live as long as core process.
// All events will be sent to STDIN without any timeout for process. But be aware of memory leaks after processing huge events.
// If plugin is exited itself core will restart its process again shortly. 

// Plugin internal specs
const pmode = "ONCE" // Can also be "STREAM"
const pjsonver = 1   // Expected JSON version
const pver = "1.0.1@u11i51pi" // You can specify everything you want. This value will show in core log when validating this plugin

var config *PluginConfig
var targetDir string // Default directory

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
func logMsg(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func main() {
	// VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
	// within 5 seconds after startup or core can throw timeout and not
	// validate your plugin for event processing.

	// Object to send handshake into stdout on start
	handshake := Handshake{
		Mode:    pmode,
		Version: pver,
		JsonVer: pjsonver,
	}

	// Send onelined JSON with \n at the end
	if err := json.NewEncoder(os.Stdout).Encode(handshake); err != nil {
		logMsg("FATAL: Error sending Handshake (%v)", err)
		os.Exit(1)
	}

	// CORE EVENTS: Core will send commands and events in STDIN of this process
	// in JSON format represented in BaseEvent struct

	// All RAM efficiency approaches below is unnecesary but recommended.
	// You can write plugin code as you wish on any programming language
	// until it matches the core requirements for validation. 

	// Using here scanner instead of json.Decoder to control buffer size
	// But you can use a simple json.Decoder as well.
	scanner := bufio.NewScanner(os.Stdin)

	// Allocate start buffer (e.g. 64KB) and set max buffer limit (e.g. 100MB)
	// This protect us from infinite RAM allocation on bad data
	const maxCapacity = 100 * 1024 * 1024 // 100MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		// scanner.Bytes() return pointer into internal scanner buffer.
		// No new memory is allocated on this line!
		rawBytes := scanner.Bytes()
		if len(rawBytes) == 0 {
			continue
		}

		// Convert into json.RawMessage type
		rawEvent := json.RawMessage(rawBytes)

		// Send RAW JSON data into your event processor
		handleEvent(rawEvent)
		// If mode is "ONCE" the program will exit with code 0 inside handleEvent function
		// And there is not so much need for RAM optimizations.

		// If mode is "STREAM" the program will wait for next event.

		// Call GC. Returning memory to OS may take some time.
		// As program has no links on scan buffer GC can clean and return some MB of RAM.
		// This approach can help us save some RAM on processed events.
		// If the handleEvent func was started inside goroutine this call of GC is useless. 
		runtime.GC()
	}

	// Check why cycle is ended
	if err := scanner.Err(); err != nil {
		logMsg("Error reading from stdin: %v", err)
	} else {
		// If scanner passes till this moment without errors then data flow is closed (EOF)
		logMsg("stdin closed by core. Shutdown.")
		os.Exit(0)
	}
}

// handleEvent decompiles event and forward data for processing
func handleEvent(raw json.RawMessage) {
	var base BaseEvent
	if err := json.Unmarshal(raw, &base); err != nil {
		logMsg("Error parsing base structure: %v", err)
		return
	}

	// Key validation requirement: If in event body "kill" is true then plugin must close its process

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
	// HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
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

	case "rkn": // Event with possible diffs to process
		saveLimit := 100 // Safe default if config still not loaded
		if config != nil && config.SaveLastEntries > 0 {
			saveLimit = config.SaveLastEntries
		}

		// Pass JSON to event parser
		processRknEvent(raw, saveLimit)

	default:
		logMsg("Unknown event type: %s", base.Type)
	}
}

// Actually you can write plugins with any purpose you want.
// You can even not process diffs or any of its data and use events as markers that
// lists on disk are updated. For updates with every diff you can read "empty":bool.
// When "empty":true that means the diff is empty. User can disable creating events if all "empty" fields are true
// As well as "empty" you can use another data described in JSON event example in core repo.

// This plugin's task: Dump every "rkn" event as readable JSON on disk

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
		// Kill process if task is done and mode "ONCE"
		os.Exit(0)
	}
}