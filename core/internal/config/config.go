package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"zapretyan-go/internal/flags"
	"zapretyan-go/internal/utils"

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

	Ver       string // App version
	AppPath   string // Specific OS path to executable directory
	Registry  map[string]string
	JsonVer   int  // Version of stdin json messages
	ExtReady  bool // Automaticly sets true when extensions has been started
	DumpEvent bool // DEBUG: Write every event json into ./debug folder

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
	DomainSource     []string // Source of Domain list
	IpSource         []string // Source of IP list
	ComDomainSources []string // Downoads all files and merge them in community.txt
}

func parseConfig() {
	defer slog.Debug("parseConfig() ended")
	// Parse config
	if _, err := toml.DecodeFile(filepath.Join(Params.AppPath, "config.toml"), RawCfg); err != nil {
		slog.Error("FATAL: Error while parsing config", "err", err)
		utils.Pause()
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
	Params.DumpEvent = flags.Args.DumpEvent
	Params.AllowCustom = GetBoolSafe(cfg, "allow_custom_extensions", false)
	Params.SendEmptyEvent = GetBoolSafe(cfg, "send_empty_report", true)
	Params.DisableIP = GetBoolSafe(cfg, "disable_ip_comparsion", false)
	Params.DisableCommunity = GetBoolSafe(cfg, "disable_community", true)

	// core.once_ctx_deadline
	octx := GetIntSafe(cfg, "once_ctx_deadline", 3600)
	if 300 > octx || octx > 2952000 {
		slog.Warn("Bad 'core.once_ctx_deadline'. Defaulting.", "min", 300, "spec", octx, "max", 2952000, "default", 3300)
		octx = 3300
	}
	Params.ExtOnceCtxTimeout = octx

	// core.report_interval
	ri := GetIntSafe(cfg, "report_interval", 1)
	if 1 > ri || ri > 720 {
		slog.Warn("Bad 'core.report_interval'. Defaulting.", "min", 1, "spec", ri, "max", 720, "default", 1)
		ri = 1
	}
	Params.ReportInterval = ri

	// core.data.data_dir
	dd := utils.GetPathState(GetStringSafe(cfg, "data.data_dir", "./data"))
	if !dd.Exists {
		slog.Info("Directory not found. Creating new", "dir", dd.AbsPath)
		err := os.MkdirAll(dd.AbsPath, 0755)
		if err != nil {
			slog.Error("FATAL: Cannot create 'core.data.data_dir'", "dir", dd.AbsPath, "err", err)
			utils.Pause()
			os.Exit(1)
		}
		dd = utils.GetPathState(dd.AbsPath)
	}

	if !dd.IsDir {
		slog.Error("FATAL: 'core.data.data_dir' IS NOT DIRECTORY!", "path", dd.AbsPath)
		utils.Pause()
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
	var defaultds = []string{"https://antifilter.download/list/domains.lst"}
	var validds []string
	domsource := GetSliceStringSafe(cfg, "data.domain_source", defaultds)
	for _, ds := range domsource {
		if !utils.IsValidURL(ds) {
			slog.Warn("Invalid URL in 'core.data.domain_source'. Key Dropped", "key", ds)
			continue
		}
		validds = append(validds, ds)
		slog.Info("", "domain_source", ds)
	}
	slog.Info("Domain sources check complete", "valid_count", len(validds))

	// Check length
	if len(validds) < 1 {
		slog.Warn("At least one URL must be valid. Defaulting Domain sources!", "source", defaultds)
		DataParams.DomainSource = defaultds
	} else {
		DataParams.DomainSource = validds
	}

	// core.data.ip_source
	var defaultips = []string{"https://antifilter.download/list/ip.lst", "https://antifilter.download/list/subnet.lst"}
	if Params.DisableIP {
		// Set default if disabled
		DataParams.IpSource = defaultips
	} else {
		var validips []string
		ipsource := GetSliceStringSafe(cfg, "data.ip_source", defaultips)
		for _, ips := range ipsource {
			if !utils.IsValidURL(ips) {
				slog.Warn("Invalid URL in 'core.data.ip_source'. Key Dropped", "key", ips)
				continue
			}
			validips = append(validips, ips)
			slog.Info("", "ip_source", ips)
		}
		slog.Info("IP sources check complete", "valid_count", len(validips))

		// Check length
		if len(validips) < 1 {
			slog.Warn("At least one URL must be valid. Defaulting IP sources!", "source", defaultips)
			DataParams.IpSource = defaultips
		} else {
			DataParams.IpSource = validips
		}
	}

	// core.data.community_domain_sources
	var validcomds []string
	comds := GetSliceStringSafe(cfg, "data.community_domain_sources", []string{})
	for _, cds := range comds {
		if Params.DisableCommunity {
			DataParams.ComDomainSources = []string{}
			break
		}
		if !utils.IsValidURL(cds) {
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

	utils.DumpStruct("Global Params State", Params)   // Output for Debug Level
	utils.DumpStruct("Data Params State", DataParams) // Output for Debug Level
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

// Returns directory where app is installed even if started through symlink
func GetAppPath() string {
	defer slog.Debug("getAppPath() ended")
	
	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		slog.Error("FATAL: Cannot get the executable path", "err", err)
		utils.Pause()
		os.Exit(1)
	}

	// If executed from symlink get real absolute path
	realExePath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		slog.Error("FATAL: Cannot evaluate symlink for executable", "err", err)
		utils.Pause()
		os.Exit(1)
	}

	// Return directory
	return filepath.Dir(realExePath)
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
	Params.AppPath = GetAppPath()

	// Change dir to this app
	if err := os.Chdir(Params.AppPath); err != nil {
		slog.Error("FATAL: Cannot change work directory to", "dir", Params.AppPath)
		utils.Pause()
		os.Exit(1)
	}

	// Get workdir for debug
	wdir, _ := os.Getwd()
	slog.Debug("", "work_dir", wdir)

	// Parse App configuration
	parseAppConfig()

	if Params.ExtOnceCtxTimeout > Params.ReportInterval * 3600 {
		slog.Error("FATAL: 'once_ctx_deadline' CANNOT BE LONGER THAN 'report_interval'! PLEASE CHECK YOUR config.toml", "once_ctx_deadline_sec", Params.ExtOnceCtxTimeout, "report_interval_sec", Params.ReportInterval * 3600)
		utils.Pause()
		os.Exit(1)
	}
}
