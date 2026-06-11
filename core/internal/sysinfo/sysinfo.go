package sysinfo

import (
	"fmt"
	"runtime"
)

// SysInfo stores collected system info
type SysInfo struct {
	OSName    string // e.g.: "Ubuntu 24.04 LTS" or "Windows 11 Pro"
	OSVersion string // Specific build e.g.: "24.04" or "10.0.22631"
}

// SendLogBanner formats start log string with build and OS info
func SendLogBanner(version string, jsonproto int) {
	// Get detailed OS info (Depending on platform)
	info := getOSInfo()

	// Formatting log string:
	// Name Version (JSON Proto ver) (Go-Version Platform) OS: Distrib/Build
	fmt.Printf("Zapretyan %s (JSON Proto v%v) (Zapretyan-Go, internet blocklist manager core) (%s %s/%s)\nOS Environment: %s (Version/Build: %s)\n",
		version,
		jsonproto,
		runtime.Version(), // Go version e.g. "go1.26.3"
		runtime.GOOS,      // Platform (windows, linux)
		runtime.GOARCH,    // Arch (amd64, arm64)
		info.OSName,
		info.OSVersion,
	)
}