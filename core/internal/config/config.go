package config

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

	Ver      string // App version
	AppPath  string // Specific OS path to executable directory
	Registry map[string]string
	JsonVer  int  // Version of stdin json messages
	ExtReady bool // Automaticly sets true when extensions has been started

	// Configuration file

	AllowCustom       bool // Allow unofficial extensions
	ReportInterval    int  // Scan and send events every n hours
	SendEmptyEvent    bool // Send Events and call ONCE Extensions even if all diffs are empty
	ExtOnceCtxTimeout int  // Force Kill ONCE Extensions on timeout
	DisableIP         bool // Disable scanning and comparing IP lists
	DisableCommunity  bool // Disable downloading and merging community lists
}

var DataParams *DataCollection

type DataCollection struct {
	DataDirectory    string   // Directory to store all data for service work
	Method           string   // "http" checks for Last-Modified tag to check difference while "hash" downloading file every time and comparing hashes
	DomainSource     string   // Source of Domain list
	IpSource         string   // Source of IP list
	ComDomainSources []string // Downoads all files and merge them in community.txt
}

// Converts slashes and add .exe suffix on windows if not specified
func ExecPath(configPath string) string {
	defer slog.Debug("ExecPath() ended")
	// Convert slashes to current OS
	fixedPath := filepath.FromSlash(configPath)

	// If Windows add ".exe" to file or ignore if alredy has executable extension
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(fixedPath))

		isExec := ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".sh"

		// If it not executable file - force add .exe
		if !isExec {
			fixedPath += ".exe"
		}
	}

	return fixedPath
}

func parseConfig() {
	defer slog.Debug("parseConfig() ended")
	// Parse config
	if _, err := toml.DecodeFile(filepath.Join(Params.AppPath, "config.toml"), RawCfg); err != nil {
		slog.Error("FATAL: Error while parsing config", "err", err)
		os.Exit(1)
	}

	// Output Debug logs in JSON
	jsonCore, err := json.MarshalIndent(RawCfg.Core, "", "  ")
	if err != nil {
		slog.Debug("ERROR converting TOML to JSON:", "err", err)
	} else {
		slog.Debug("--- [DEBUG] CORE TOML INTO JSON ---")
		slog.Debug(string(jsonCore))
		slog.Debug("--- [JSON DEBUG END] ---")
	}
}

func parseAppConfig() {
	defer slog.Debug("parseAppConfig() ended")
	slog.Info("Reading core config...")
	parseConfig()
	cfg := RawCfg.Core

	// core.*
	Params.AllowCustom = GetBoolSafe(cfg, "allow_custom_extensions", false)
	Params.SendEmptyEvent = GetBoolSafe(cfg, "send_empty_report", true)
	Params.ExtOnceCtxTimeout = GetIntSafe(cfg, "once_ctx_deadline", 3600)
	Params.DisableIP = GetBoolSafe(cfg, "disable_ip_comparsion", false)
	Params.DisableCommunity = GetBoolSafe(cfg, "disable_community", true)

	// core.report_interval
	ri := GetIntSafe(cfg, "report_interval", 1)
	if 1 > ri || ri > 720 {
		ri = 1
	}
	Params.ReportInterval = ri

	// core.data.data_dir
	dd := GetPathState(GetStringSafe(cfg, "data.data_dir", "./data"))
	if !dd.Exists {
		slog.Info("Directory not found. Creating new", "dir", dd.AbsPath)
		err := os.MkdirAll(dd.AbsPath, 0755)
		if err != nil {
			slog.Error("FATAL: Cannot create 'core.data.data_dir'", "dir", dd.AbsPath, "err", err)
			os.Exit(1)
		}
		dd = GetPathState(dd.AbsPath)
	}

	if !dd.IsDir {
		slog.Error("FATAL: 'core.data.data_dir' IS NOT DIRECTORY!", "path", dd.AbsPath)
		os.Exit(1)
	}
	DataParams.DataDirectory = dd.AbsPath

	// core.data.method
	m := GetStringSafe(cfg, "data.method", "http")
	if m != "http" && m != "hash" {
		slog.Warn("BAD PARAM IN 'core.data.method'. Fallback to HTTP.")
		m = "http"
	}
	DataParams.Method = m

	// core.data.domain_source
	ds := GetStringSafe(cfg, "data.domain_source", "https://antifilter.download/list/domains.lst")
	if !IsValidURL(ds) {
		slog.Warn("BAD URL IN 'core.data.domain_source'. Fallback to default")
		ds = "https://antifilter.download/list/domains.lst"
	}
	DataParams.DomainSource = ds
	slog.Info("", "domain_source", ds)

	// core.data.ip_source
	is := GetStringSafe(cfg, "data.ip_source", "https://antifilter.download/list/ip.lst")
	if !IsValidURL(is) {
		slog.Warn("BAD URL IN 'core.data.ip_source'. Fallback to default")
		is = "https://antifilter.download/list/ip.lst"
	}
	DataParams.IpSource = is
	slog.Info("", "ip_source", is)

	// core.data.community_domain_sources
	var validcomds []string
	comds := GetSliceStringSafe(cfg, "data.community_domain_sources", []string{})
	for _, cds := range comds {
		if Params.DisableCommunity {
			DataParams.ComDomainSources = []string{}
			break
		}
		if !IsValidURL(cds) {
			slog.Warn("Invalid URL in 'core.data.community_domain_sources'. Key Dropped", "key", cds)
			continue
		}
		validcomds = append(validcomds, cds)
		slog.Info("", "community_source", cds)
	}
	slog.Info("Community sources check complete", "valid_count", len(validcomds))
	if !Params.DisableCommunity && len(validcomds) < 1 {
		slog.Error("At least one URL must be valid. Disabling feature 'Community'!")
		DataParams.ComDomainSources = []string{}
		Params.DisableCommunity = true
	}
	if !Params.DisableCommunity {
		DataParams.ComDomainSources = validcomds
	}

	DumpStruct("Global Params State", Params)   // Output for Debug Level
	DumpStruct("Data Params State", DataParams) // Output for Debug Level
}

// PathState describe full state of path
type PathState struct {
	Exists       bool      // Is exist on disk?
	IsDir        bool      // Is it Directory?
	IsFile       bool      // Is it File?
	IsExecutable bool      // Can we execute it?
	AbsPath      string    // Normalized absolute Path
	ModTime      time.Time // Time when file was modified (Use with .UTC())
}

// GetPathState doing complex validation of pathstring
func GetPathState(rawPath string) PathState {
	defer slog.Debug("GetPathState() ended")
	var state PathState

	// Normalize path (Convert all ./ ../ to real path)
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		// If we cannot get abs path then syntax is broken
		return state
	}
	state.AbsPath = abs

	// Request metadata from OS
	info, err := os.Stat(abs)
	if err != nil {
		// If file not exist - return struct with Exists = false
		if os.IsNotExist(err) {
			return state
		}
		// Other errors (e.g. Access Denied) means that file exists but we cannot read it
		return state
	}

	// If all fine then fill base flags
	state.Exists = true
	state.IsDir = info.IsDir()
	state.IsFile = !info.IsDir()

	// Get last modified date
	state.ModTime = info.ModTime()

	// Check if executable (Files only)
	if state.IsFile {
		if runtime.GOOS == "windows" {
			// On Windows check file extension
			ext := strings.ToLower(filepath.Ext(abs))
			state.IsExecutable = ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".sh"
		} else {
			// On Linux/macOS check POSIX rights bits (+x for owner/group/everyone)
			state.IsExecutable = info.Mode()&0111 != 0
		}
	}

	return state
}

func IsValidURL(s string) bool {
	defer slog.Debug("IsValidURL() ended")
	u, err := url.ParseRequestURI(s)
	slog.Debug("URL Test", "url", s, "scheme", u.Scheme, "err", err)
	if err != nil {
		return false // String is not valid URI
	}
	// Check that proto is http(s)
	return u.Scheme == "http" || u.Scheme == "https"
}

// GetBoolSafe returns default value if key is wrong or missing
func GetBoolSafe(m map[string]interface{}, key string, defaultVal bool) bool {
	defer slog.Debug("GetBoolSafe() ended")
	if b, ok := getNestedValue(m, key); ok {
		if val, ok := b.(bool); ok {
			slog.Debug("Parsed value", "k", key, "v", val)
			return val
		}
	}
	slog.Warn("Failed to parse. Set to default", "key", key, "default", defaultVal)
	return defaultVal
}

// GetIntSafe safely gets integers (TOML parses integers as int64)
func GetIntSafe(m map[string]interface{}, key string, defaultVal int) int {
	defer slog.Debug("GetIntSafe() ended")
	if i, ok := getNestedValue(m, key); ok {
		if val, ok := i.(int64); ok {
			slog.Debug("Parsed value", "k", key, "v", val)
			return int(val)
		}
	}
	slog.Warn("Failed to parse. Set to default", "key", key, "default", defaultVal)
	return defaultVal
}

// GetStringSafe safely gets strings
func GetStringSafe(m map[string]interface{}, key string, defaultVal string) string {
	defer slog.Debug("GetStringSafe() ended")
	if s, ok := getNestedValue(m, key); ok {
		if val, ok := s.(string); ok {
			slog.Debug("Parsed value", "k", key, "v", val)
			return val
		}
	}
	slog.Warn("Failed to parse. Set to default", "key", key, "default", defaultVal)
	return defaultVal
}

// GetSliceStringSafe with conversion []interface{} into []string
func GetSliceStringSafe(m map[string]interface{}, key string, defaultVal []string) []string {
	defer slog.Debug("GetSliceStringSafe() ended")
	val, ok := getNestedValue(m, key)
	if !ok {
		slog.Warn("Slice key not found. Set to default", "key", key)
		return defaultVal
	}

	// If parser returns clean []string
	if slice, ok := val.([]string); ok {
		return slice
	}

	// If parser returns []interface{}
	if interfaceSlice, ok := val.([]interface{}); ok {
		res := make([]string, 0, len(interfaceSlice))
		for _, item := range interfaceSlice {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		slog.Debug("Parsed []string", "k", key, "count", len(res))
		return res
	}

	slog.Warn("Failed to parse []string. Set to default", "key", key)
	return defaultVal
}

// Read value on deeper levels
func getNestedValue(m map[string]interface{}, key string) (interface{}, bool) {
	defer slog.Debug("getNestedValue() ended")
	parts := strings.Split(key, ".")
	var current interface{} = m

	for _, part := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		var exists bool
		current, exists = currentMap[part]
		if !exists {
			return nil, false
		}
	}
	return current, true
}

// Returns directory where app is installed
func getAppPath() string {
	defer slog.Debug("getAppPath() ended")
	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return "" // Fallback to workdir on fail
	}

	// Get only directory
	return filepath.Dir(exePath)
}

// Universal Helper for output debug structure in console
func DumpStruct(title string, v interface{}) {
	defer slog.Debug("DumpStruct() ended")
	slog.Debug("--- [DEBUG DUMP] ---", "name", title)

	// Protection: If nil pointer output nil for debug
	if v == nil {
		slog.Debug("<nil> (Structure not initialized)")
		return
	}

	bytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		slog.Debug("ERROR formatting JSON", "err", err)
		return
	}
	slog.Debug(string(bytes))
	slog.Debug("--- [END OF DUMP] ---")
}

func InitConfig() {
	defer slog.Debug("InitConfig() ended")
	// Init params
	Params = &GlobalParams{}
	DataParams = &DataCollection{}
	RawCfg = &RootConfig{}

	Params.Registry = map[string]string{
		"Discord Sender":   "@36hbsug1",
		"Daily Statistics": "@f61tsi7f",
		"File Logger":      "@u11i51pi",
	}

	Params.ExtReady = false

	// Get Application Directory
	Params.AppPath = getAppPath()

	// Parse App configuration
	parseAppConfig()
}
