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
	Ver 		string
	AppPath 	string
	Registry 	map[string]string
	AllowCustom bool
	JsonVer 	int
}

func preparePath(configPath string) string {
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
		slog.Error("Error while parsing config", "err", err)
		os.Exit(1)
	}
}

func parseAppConfig() {
// TODO
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

	// Get Application Directory
	Params.AppPath = getAppPath()
}