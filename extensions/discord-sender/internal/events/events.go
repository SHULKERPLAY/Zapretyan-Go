package events

import (
	"discord-sender/internal/cfg"
)

// Handshake is a structure of first reply to core
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

// BaseEvent - Base structure to understand what delivered without full parse
type BaseEvent struct {
	Ver  int           `json:"ver"`  // Core JSON format version
	Type string        `json:"type"` // "cmd" for commands and config send, "rkn" for events
	Kill bool          `json:"kill"` // If true process must exit immidiately
	Path string        `json:"path"` // Absolute path to Data directory
	Diff *DiffObject   `json:"diff"` // Diff container. If empty it will be nil
	Cfg  *cfg.PluginConfig `json:"cfg"`  // Field can be empty so use the pointer. If empty it will be nil
}

// Structure for every object in "diff"
type DiffObject struct {
	BannedDomains   DiffObjectData `json:"banned"`      // Banned domains
	UnbannedDomains DiffObjectData `json:"unbanned"`    // Unbanned domains
	BannedIPs       DiffObjectData `json:"banned_ip"`   // Banned IP
	UnbannedIPs     DiffObjectData `json:"unbanned_ip"` // Unbanned IP
}

// Structure for data objects inside "diff"
type DiffObjectData struct {
	Empty  bool     `json:"empty"`  // If true then Data empty
	Length int      `json:"length"` // Length of Data array
	Data   []string `json:"data"`   // All changes
	Total  int      `json:"total"`  // Total count of type
}