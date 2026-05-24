package listwriter

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"zapretyan-go/internal/diffprocess"
	"zapretyan-go/internal/downloader"
)

// ListDownloadAndMerge downloading all files from array and merge them with deduplication, sorts and renaming.
// Returns bool where true on success and false if operation was failed.
// Wait as lists "domain", "ip" or "community"
func ListDownloadAndMerge(ctx context.Context, wg *sync.WaitGroup, urls []string, targetDir, finalTmpPath string, lists string) bool {
	defer slog.Debug("ListDownloadAndMerge() ended")
	defer wg.Done() // Report that function is ended

	if lists != "domain" && lists != "ip" && lists != "community" {
		slog.Error("Invalid merge type", "type", lists)
		return false
	}

	tmpFiles, err := downloader.DownloadArray(ctx, urls, targetDir, finalTmpPath)
	if err != nil {
		slog.Error("Error while downloading. Process canceled", "list", lists, "err", err)
		return false
	}

	// Count possible lines count to allocate memory for map.
	// It needed for RAM optimization
	var length int
	for _, file := range tmpFiles {
		lin, _ := diffprocess.CountNonEmptyLines(file)
		length = length + lin
	}

	// Merge all lines into map.
	// They will be deduplicated as map can store only unique keys
	linesMap := make(map[string]struct{}, length)

	for _, file := range tmpFiles {
		// Read all lines to map. Lines deduplicated automaticly
		// because structure map[string]struct{} can store only unique keys
		if err := ReadLinesToMap(file, linesMap); err != nil {
			slog.Error("Error reading tmp file", "file", file, "err", err)
			return false
		}
	}  

	if lists == "ip" {
		UnpackCIDRInMap(linesMap)
	}

	// Create map to transfer linemap to array of strings.
	// The reason is that we cannot sort the map.
	// So map is transfered to sort and write sorted data to file.
	uniqueLines := make([]string, 0, len(linesMap))
	for line := range linesMap {
		uniqueLines = append(uniqueLines, line)
	}
	sort.Strings(uniqueLines)

	// Write all data to list.tmp
	if err := WriteLinesToFile(finalTmpPath, uniqueLines); err != nil {
		slog.Error("Error writing lists", "file", finalTmpPath, "err", err)
		return false
	}

	return true
}

// UnpackCIDRInMap finds all CIDR-prefixes in map, unpacking them as single IP and remove prefixes.
func UnpackCIDRInMap(linesMap map[string]struct{}) {
	defer slog.Debug("UnpackCIDRInMap() ended")
	// Create array to store prefixes which need to be removed.
	// Allocate memory one time to avoid reallocations.
	var keysToDelete []string

	// Temporary map for new IPs to not modify linesMap in iterations
	detectedIPs := make(map[string]struct{})

	for key := range linesMap {
		// Quick check: if line not exist or do not have mask then skip it
		if !strings.ContainsRune(key, '/') {
			continue
		}

		// Parse CIDR with netip (zero-allocation parser)
		prefix, err := netip.ParsePrefix(key)
		if err != nil {
			// If line broken or not parse as CIDR skip it
			continue
		}

		// Store prefix to delete it after
		keysToDelete = append(keysToDelete, key)

		// Get first IP and mask address
		addr := prefix.Addr()

		// Iterating by all IP addresses inside subnet
		// Prefix /32 (IPv4) or /128 (IPv6) returns only one address
		for prefix.Contains(addr) {
			detectedIPs[addr.String()] = struct{}{}
			addr = addr.Next()

			// Protect from infinite cycle
			if addr == (netip.Addr{}) {
				break
			}
		}
	}

	// Remove old keys with prefixes
	for _, key := range keysToDelete {
		delete(linesMap, key)
	}

	// Move unpacked IPs to main map
	for ip := range detectedIPs {
		linesMap[ip] = struct{}{}
	} // No need to return objects. Map data edited in Go directly without the pointers
}

// Reading file lines to map for deduplication
func ReadLinesToMap(path string, linesMap map[string]struct{}) error {
	defer slog.Debug("readLinesToMap() ended")
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Get bytes without allocating memory on heap
		bytesLine := bytes.TrimSpace(scanner.Bytes())
		if len(bytesLine) == 0 {
			continue
		}

		// Allocate memory only when writing to map.
		linesMap[string(bytesLine)] = struct{}{}
	}
	return scanner.Err()
}

// Write slice of lines to file
func WriteLinesToFile(path string, lines []string) error {
	defer slog.Debug("writeLinesToFile() ended")

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		// Write base string
		if _, err := writer.WriteString(line); err != nil {
			return err
		}
		// Write newline symbol as one byte
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}

	// Flushing buffer to file
	return writer.Flush()
}
