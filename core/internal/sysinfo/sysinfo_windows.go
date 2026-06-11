//go:build windows

package sysinfo

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

func getOSInfo() SysInfo {
	info := SysInfo{
		OSName:    "Windows",
		OSVersion: "Unknown Build",
	}

	// Open registry and read windows build info
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return info
	}
	defer k.Close()

	// Get public OS name (e.g. "Windows 11 Pro")
	pn, _, err := k.GetStringValue("ProductName")
	if err == nil {
		info.OSName = pn
	}

	// In Windows 10/11 ProductName can continue write "Windows 10", 
	// Identify system by its CurrentBuild
	build, _, err := k.GetStringValue("CurrentBuild")
	if err == nil {
		info.OSVersion = build // e.g. 22631
	}

	// Get DisplayVersion (e.g. "23H2")
	displayVer, _, err := k.GetStringValue("DisplayVersion")
	if err == nil && displayVer != "" {
		info.OSVersion = fmt.Sprintf("%s (%s Update)", info.OSVersion, displayVer)
	}

	return info
}