package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// Raw Root config struct
var RawCfg *RootConfig

type RootConfig struct {
	Core       map[string]interface{}   `toml:"core"`
	Extensions []map[string]interface{} `toml:"extension"`
}

var Params *GlobalParams

type GlobalParams struct {
	// Internal statements

	Ver 		string 				// App version
	AppPath 	string 				// Specific OS path to executable directory
	Registry 	map[string]string
	JsonVer 	int 				// Version of stdin json messages
	ExtReady 	bool 				// Automaticly sets true when extensions has been started

	// Configuration file

	AllowCustom       bool // Allow unofficial extensions
	ReportInterval    int  // Scan and send events every n hours
	SendEmptyEvent    bool // Send Events and call ONCE Extensions even if all diffs are empty
	ExtOnceCtxTimeout int  // Force Kill ONCE Extensions on timeout
	DisableIP 		  bool // Disable scanning and comparing IP lists
	DisableCommunity  bool // Disable downloading and merging community lists
}

var DataParams *GlobalParams

type DataCollection struct {
	DataDirectory    string	  // Directory to store all data for service work 
	Method 			 string	  // "http" checks for Last-Modified tag to check difference while "hash" downloading file every time and comparing hashes
	DomainSource 	 string	  // Source of Domain list
	IpSource 		 string	  // Source of IP list
	ComDomainSources []string // Downoads all files and merge them in community.txt
}

func ExecPath(configPath string) string {
	// Convert slashes to current OS
	fixedPath := filepath.FromSlash(configPath)

	// If Windows add ".exe" file extension or ignore if alredy has ".exe"
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(strings.ToLower(fixedPath), ".exe") {
			fixedPath += ".exe"
		}
	}

	return fixedPath
}

func parseConfig() {
	// Parse config
	if _, err := toml.DecodeFile(filepath.Join(Params.AppPath,"config.toml"), RawCfg); err != nil {
		slog.Error("FATAL: Error while parsing config", "err", err)
		os.Exit(1)
	}
}

func parseAppConfig() {
	slog.Info("Reading core config...")
	cfg := RawCfg.Core

	Params.AllowCustom = GetBoolSafe(cfg, "allow_custom_extensions", false)

	ri := GetIntSafe(cfg, "report_interval", 1)
	if 1 > ri || ri > 720 { ri = 1 }
	Params.ReportInterval = ri

	Params.SendEmptyEvent = GetBoolSafe(cfg, "send_empty_report", true)
	Params.ExtOnceCtxTimeout = GetIntSafe(cfg, "once_ctx_deadline", 3600)
	Params.DisableIP = GetBoolSafe(cfg, "disable_ip_comparsion", false)
 	Params.DisableCommunity = GetBoolSafe(cfg, "disable_community", true)

	// in progress
}

// GetBoolSafe returns default value if key is wrong or missing
func GetBoolSafe(m map[string]interface{}, key string, defaultVal bool) bool {
	if val, ok := m[key].(bool); ok {
		slog.Debug("Parsed value", "key", key, "value", val)
		return val
	}
	slog.Warn("Failed to parse. Set to default", "key", key, "default", defaultVal)
	return defaultVal
}

// GetIntSafe safely gets integers (TOML parses integers as int64)
func GetIntSafe(m map[string]interface{}, key string, defaultVal int) int {
	if val, ok := m[key].(int64); ok {
		slog.Debug("Parsed value", "key", key, "value", val)
		return int(val)
	}
	slog.Warn("Failed to parse. Set to default", "key", key, "default", defaultVal)
	return defaultVal
}

// GetStringSafe safely gets strings
func GetStringSafe(m map[string]interface{}, key string, defaultVal string) string {
	if val, ok := m[key].(string); ok {
		slog.Debug("Parsed value", "key", key, "value", val)
		return val
	}
	slog.Warn("Failed to parse. Set to default", "key", key, "default", defaultVal)
	return defaultVal
}

// Returns directory where app is installed
func getAppPath() string {
	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return "" // Fallback to workdir on fail
	}

	// Get only directory
	return filepath.Dir(exePath)
}

func init() {
	// Init params
	Params = &GlobalParams{}
	RawCfg = &RootConfig{}

	// Version of JSON message payload
	Params.JsonVer = 1

	Params.Registry = map[string]string{
		"Discord Sender":   "@36hbsug1",
		"Daily Statistics": "@f61tsi7f",
		"File Logger":      "@u11i51pi",
	}

	Params.ExtReady = false

	// Get Application Directory
	Params.AppPath = getAppPath()
}