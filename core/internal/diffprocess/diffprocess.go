package diffprocess

import (
	"bufio"
	"bytes"
	"errors"
	"log/slog"
	"os"
	"sort"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/utils"
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

	// Check if we need to try sort files for this diff
	if filestate := utils.GetPathState(oldFile); !filestate.Exists {
		slog.Warn("Old file not found. Diff empty until new rotation arrives.", "file", oldFile)
		return result
	}

	// Sort source files on disk into temporary files.
	// We can get rid of map for sort if we perform it on disk.
	sortedOldPath, err := sortFileOnDisk(oldFile)
	if err != nil {
		slog.Error("Failed to sort old file", "err", err)
		return result
	}
	defer os.Remove(sortedOldPath)

	sortedNewPath, err := sortFileOnDisk(newFile)
	if err != nil {
		slog.Error("Failed to sort new file", "err", err)
		return result
	}
	defer os.Remove(sortedNewPath)

	// Open both sored files
	fOld, err := os.Open(sortedOldPath)
	if err != nil {
		return result
	}
	defer fOld.Close()

	fNew, err := os.Open(sortedNewPath)
	if err != nil {
		return result
	}
	defer fNew.Close()

	scOld := bufio.NewScanner(fOld)
	scNew := bufio.NewScanner(fNew)

	// Read first lines
	hasOld := scOld.Scan()
	hasNew := scNew.Scan()

	// Merge algorythm (Two Pointers). Store in RAM only two lines.
	for hasOld && hasNew {
		oldLine := scOld.Bytes()
		newLine := scNew.Bytes()

		cmp := bytes.Compare(oldLine, newLine)
		if cmp == 0 {
			// Lines identical. Duplicate - scan next lines
			hasOld = scOld.Scan()
			hasNew = scNew.Scan()
		} else if cmp < 0 {
			// String from oldFile less then it gone in newFile. It deleted.
			// Scan next line only in old file
			result.Removed = append(result.Removed, string(oldLine))
			hasOld = scOld.Scan()
		} else {
			// String from newFile less then it gone in oldFile. It added.
			// Scan next line only in new file
			result.Added = append(result.Added, string(newLine))
			hasNew = scNew.Scan()
		}
	}

	// Collect rest if one file ended before another
	for hasOld {
		// Works only if new file is ended
		result.Removed = append(result.Removed, scOld.Text())
		hasOld = scOld.Scan()
	}
	for hasNew {
		// Works only if old file is ended
		result.Added = append(result.Added, scNew.Text())
		hasNew = scNew.Scan()
	}

	// Check for errors
	if err := scOld.Err(); err != nil {
		slog.Error("An error has occured in diffscanner", "err", err)
	}
	if err := scNew.Err(); err != nil {
		slog.Error("An error has occured in diffscanner", "err", err)
	}

	// No need to sort final slices as they appended already sorted!
	return result 
}

// sortFileOnDisk execute External Merge Sort.
// RAM hardlimited by chunkSizeLimit.
func sortFileOnDisk(srcPath string) (string, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var chunkPaths []string
	var memoryBuffer []string
	const chunkSizeLimit = 100000 // 100000 lines RAM buffer limit

	// Helper for write buffer on disk
	dumpBuffer := func() error {
		if len(memoryBuffer) == 0 {
			return nil
		}
		// Sort chunk in memory
		sort.Strings(memoryBuffer)

		// Write chunk on disk
		// Creating file
		tmp, err := os.CreateTemp(config.DataParams.DataDirectory, "sort-chunk-*.tmp")
		if err != nil {
			return err
		}
		defer tmp.Close()

		// Write range in bytes to save on RAM
		writer := bufio.NewWriter(tmp)
		for _, line := range memoryBuffer {
			writer.WriteString(line)
			writer.WriteByte('\n')
		}
		writer.Flush()

		// Save path of current chunk
		chunkPaths = append(chunkPaths, tmp.Name())
		memoryBuffer = memoryBuffer[:0] // Flush buffer saving allocated space
		return nil
	}

	// Scan lines
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		memoryBuffer = append(memoryBuffer, scanner.Text())
		if len(memoryBuffer) >= chunkSizeLimit {
			// Buffer overflow. Dump to tmp file
			if err := dumpBuffer(); err != nil {
				return "", err
			}
		}
	}
	
	// Dump rest on disk
	if err := dumpBuffer(); err != nil {
		return "", err
	}
	
	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// If no chunks (empty file)
	if len(chunkPaths) == 0 {
		emptyTmp, _ := os.CreateTemp(config.DataParams.DataDirectory, "sort-empty-*.tmp")
		emptyTmp.Close()
		return emptyTmp.Name(), nil
	}

	// Tree merge for sorted chunks
	// Merge files by two until only one file left
	for len(chunkPaths) > 1 {
		var nextRoundChunks []string

		// Start with iteration 0 compare it to chunks count and increase counter by 2 (as two files merged)
		for i := 0; i < len(chunkPaths); i += 2 {
			// If iteration + 1 more than count of chunks
			if i+1 < len(chunkPaths) {
				// Merge two chunks in one new
				mergedPath, err := mergeTwoFiles(chunkPaths[i], chunkPaths[i+1])
				if err != nil {
					return "", err
				}

				// Send new chunk to next round of merge
				nextRoundChunks = append(nextRoundChunks, mergedPath)

				// Remove processed old chunks
				os.Remove(chunkPaths[i])
				os.Remove(chunkPaths[i+1])
			} else {
				// Odd chunk passes to next round without merging
				nextRoundChunks = append(nextRoundChunks, chunkPaths[i])
			}
		}

		// Define next round of merging
		chunkPaths = nextRoundChunks
	}

	return chunkPaths[0], nil
}

// mergeTwoFilesreading two sorted files line by line and merges them into one.
// Not supports deduplication.
func mergeTwoFiles(file1, file2 string) (string, error) {
	// Open file one
	f1, err := os.Open(file1)
	if err != nil {
		return "", err
	}
	defer f1.Close()

	// Open file two
	f2, err := os.Open(file2)
	if err != nil {
		return "", err
	}
	defer f2.Close()

	// Create temporary merged file
	out, err := os.CreateTemp(config.DataParams.DataDirectory, "sort-merge-*.tmp")
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Start two scanners
	sc1 := bufio.NewScanner(f1)
	sc2 := bufio.NewScanner(f2)
	writer := bufio.NewWriter(out)

	has1 := sc1.Scan()
	has2 := sc2.Scan()

	// Two Pointers method. Store in RAM only two lines.
	for has1 && has2 {
		b1 := sc1.Bytes()
		b2 := sc2.Bytes()

		if bytes.Compare(b1, b2) <= 0 {
			// Lines identical or less (not existing in file 2).
			// Write from file 1 - scan next line of file 1
			writer.Write(b1)
			writer.WriteByte('\n')
			has1 = sc1.Scan()
		} else {
			// Line from file 2 less then it not existing in file 1.
			// Write from file 2 - scan next line of file 2
			writer.Write(b2)
			writer.WriteByte('\n')
			has2 = sc2.Scan()
		}
	}

	// Write rest if one file ended before another
	for has1 {
		// Works only if file 2 is ended
		writer.Write(sc1.Bytes())
		writer.WriteByte('\n')
		has1 = sc1.Scan()
	}
	for has2 {
		// Works only if file 1 is ended
		writer.Write(sc2.Bytes())
		writer.WriteByte('\n')
		has2 = sc2.Scan()
	}

	// Flush data on disk from buffer
	writer.Flush()

	// Check for errors
	if err := sc1.Err(); err != nil {
		slog.Error("An error has occured in scanner while sorting temporary files", "err", err)
		return "", err
	}
	if err := sc2.Err(); err != nil {
		slog.Error("An error has occured in scanner while sorting temporary files", "err", err)
		return "", err
	}

	return out.Name(), nil
}

// RotateFiles execute fast file rotation by chain:
// 1. new.txt -> old.txt (old.txt overwritten)
// 2. new.tmp -> new.txt
// 3. newip.txt -> oldip.txt (oldip.txt overwritten)
// 4. newip.tmp -> newip.txt
// isDomain, isIp toggles rotation for types of files
func RotateFiles(newTxt, newIpTxt, communityTxt, newTmp, newIpTmp, communityTmp, oldTxt, oldIpTxt string, isDomain, isIp, isCommunity bool) {
	defer slog.Debug("RotateFiles() ended")

	slog.Info("Preparing file rotation", "domains", isDomain, "ips", isIp, "community", isCommunity)
	// If domains are successfuly downloaded and has changes
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
	// If ips are successfuly downloaded and has changes
	if isIp {
		if err := RenameIfExists(newIpTxt, oldIpTxt); err != nil {
			slog.Error("Error rotating main file into old", "from", newTxt, "to", oldIpTxt, "err", err)
		}
		if err := RenameIfExists(newIpTmp, newIpTxt); err != nil {
			slog.Error("Error activating main file", "from", newIpTmp, "to", newIpTxt, "err", err)
		}
	}

	if isCommunity {
		if err := RenameIfExists(communityTmp, communityTxt); err != nil {
			slog.Error("Error activating main file", "from", communityTmp, "to", communityTxt, "err", err)
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

	// Protection from very long lines (Limit by 1MB)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		// bytes.TrimSpace работает напрямую с байтами исходного буфера без выделения памяти
		if len(bytes.TrimSpace(scanner.Bytes())) > 0 {
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("An error has occured in total length counter. Defaulting to 0", "err", err)
		return 0, err
	}

	return count, nil
}