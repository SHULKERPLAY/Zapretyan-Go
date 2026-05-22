package diffprocess

import (
	"bufio"
	"errors"
	"log/slog"
	"os"
	"sort"
	"strings"
)

// Struct combines data about additions and deletions arrays for IPs and Domains
type RawDiff struct {
	Domain DiffResult // Data about domain changes
	Ip     DiffResult // Data about IPs changes
}

// Accepts filepath of new and old domain lists and ip lists and boolean switches for Domain and IP
// Return struct with data aboud additions and deletions in Domain and Ip lists
// If boolean switch false - return empty array
func CheckDiff(newDomain, oldDomain, newIp, oldIp string, isDomain, isIp bool) RawDiff {
	defer slog.Debug("CheckDiff() ended")

	// Initialize Domain data
	domres := DiffResult{
		Added:   []string{},
		Removed: []string{},
	}

	// Initialize IP data
	ipres := DiffResult{
		Added:   []string{},
		Removed: []string{},
	}

	// Check domain toggle and compute diff
	if isDomain {
		domres = fileDiff(oldDomain, newDomain)
	}

	// Check IP toggle and compute diff
	if isIp {
		ipres = fileDiff(oldIp, newIp)
	}

	result := RawDiff{
		Domain: domres,
		Ip:     ipres,
	}

	slog.Info("Differences defined")
	return result
}

// To Store additions and deletions between 2 files
type DiffResult struct {
	Added   []string // Array of additions
	Removed []string // Array of deletions
}

func fileDiff(oldFile, newFile string) DiffResult {
	defer slog.Debug("FileDiff() ended")

	result := DiffResult{
		Added:   []string{},
		Removed: []string{},
	}

	// Read old file and write to frequency map
	// int needs if file has duplicate strings
	oldLines := make(map[string]int)

	fOld, err := os.Open(oldFile)
	if err != nil {
		slog.Error("Error opening file", "file", oldFile)
		return result
	}
	defer fOld.Close()

	scannerOld := bufio.NewScanner(fOld)
	for scannerOld.Scan() {
		oldLines[scannerOld.Text()]++
	}
	if err := scannerOld.Err(); err != nil {
		slog.Error("File scanner error", "file", oldFile, "err", err)
	}

	// Read new file and compare
	fNew, err := os.Open(newFile)
	if err != nil {
		slog.Error("Error opening file", "file", newFile, "err", err)
		return result
	}
	defer fNew.Close()

	scannerNew := bufio.NewScanner(fNew)
	for scannerNew.Scan() {
		line := scannerNew.Text()

		if count, exists := oldLines[line]; exists && count > 0 {
			// If string in both lines - reduce counter
			oldLines[line]--
		} else {
			// String not existing in old file. Count as addition
			result.Added = append(result.Added, line)
		}
	}
	if err := scannerNew.Err(); err != nil {
		slog.Error("File scanner error", "file", newFile, "err", err)
	}

	// All that left in oldLines with counter > 0 is deleted strings
	for line, count := range oldLines {
		for i := 0; i < count; i++ {
			result.Removed = append(result.Removed, line)
		}
	}

	// Sort srtings ascending
	sort.Strings(result.Added)
	sort.Strings(result.Removed)

	return result
}

// RotateFiles execute fast file rotation by chain:
// 1. new.txt -> old.txt (old.txt overwritten)
// 2. new.tmp -> new.txt
// 3. newip.txt -> oldip.txt (oldip.txt overwritten)
// 4. newip.tmp -> newip.txt
// isDomain, isIp toggles rotation for types of files
func RotateFiles(newTxt, newIpTxt, oldTxt, oldIpTxt, newTmp, newIpTmp string, isDomain, isIp bool) {
	defer slog.Debug("RotateFiles() ended")

	if isDomain {
		// Move existing files to old (No need to delete them. os.Rename will overwrite old files)
		if err := RenameIfExists(newTxt, oldTxt); err != nil {
			slog.Error("Error rotating main file into old", "from", newTxt, "to", oldTxt, "err", err)
		}
		// Move temporary files as main ones
		if err := RenameIfExists(newTmp, newTxt); err != nil {
			slog.Error("Error activating main file", "from", newTmp, "to", newTxt, "err", err)
		}
	}

	if isIp {
		if err := RenameIfExists(newIpTxt, oldIpTxt); err != nil {
			slog.Error("Error rotating main file into old", "from", newTxt, "to", oldTxt, "err", err)
		}
		if err := RenameIfExists(newIpTmp, newIpTxt); err != nil {
			slog.Error("Error activating main file", "from", newIpTmp, "to", newIpTxt, "err", err)
		}
	}

	slog.Info("File rotation completed")
}

// Helper that renames file ignoring error if source file not exists.
func RenameIfExists(from, to string) error {
	defer slog.Debug("renameIfExists() ended")
	err := os.Rename(from, to)
	if err != nil {
		// If file "from" not exist, ignore.
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// CountNonEmptyLines returns amount of non-empty lines in file.
func CountNonEmptyLines(filePath string) (int, error) {
	defer slog.Debug("CountNonEmptyLines() ended")

	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		// strings.TrimSpace removes spaces, tabs and newlines
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}

	// Check for buffer read errors
	if err := scanner.Err(); err != nil {
		slog.Error("An error has occured in total length counter. Defaulting to 0")
		return 0, err
	}

	return count, nil
}
