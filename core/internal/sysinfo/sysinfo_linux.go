//go:build linux

package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func getOSInfo() SysInfo {
	info := SysInfo{
		OSName:    "Linux",
		OSVersion: "Unknown Kernel",
	}

	// Read default system distro identety file
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return info
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		// Removing "" if found
		val := strings.Trim(parts[1], ` "`)

		switch key {
		case "NAME":
			info.OSName = val // e.g. Ubuntu, Debian, Alpine Linux
		case "VERSION_ID":
			info.OSVersion = val // e.g. 24.04, 12, 3.20
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		fmt.Printf("Error scanning os distro")
	}

	return info
}