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
	"zapretyan-go/internal/downloader"
)

// Lines limit to dump on disk.
// We store in RAM only 100000 lines 
// to not jump above 20MB of self allocation.
const chunkSizeLimit = 100000

// ListDownloadAndMerge downloads files and unpacking found CIDRs in IP mode.
// Using external sort to not allocate too much RAM for sort only.
func ListDownloadAndMerge(ctx context.Context, urls []string, targetDir, finalTmpPath string, lists string) bool {
	defer slog.Debug("ListDownloadAndMerge() ended")

	// If not one of supported modes log error end return
	if lists != "domain" && lists != "ip" && lists != "community" {
		slog.Error("Invalid merge type", "type", lists)
		return false
	}

	// Downloading array of urls to merge
	tmpFiles, err := downloader.DownloadArray(ctx, urls, targetDir, finalTmpPath)
	if err != nil {
		slog.Error("Error while downloading. Process canceled", "list", lists, "err", err)
		return false
	}

	// Read all files, unpacking CIDRs in process and write in sorted chunks
	var chunkPaths []string
	var memoryBuffer []string

	// Helper for write buffer on disk
	dumpBufferToDisk := func() error {
		if len(memoryBuffer) == 0 {
			return nil
		}

		// Sort chunk in memory
		sort.Strings(memoryBuffer)

		// Deduplicate lines inside chunk to save on disk space
		compacted := make([]string, 0, len(memoryBuffer))
		var lastStr string
		for i, str := range memoryBuffer {
			if i == 0 || str != lastStr {
				compacted = append(compacted, str)
				lastStr = str
			}
		}

		// Write chunk on disk
		// Creating file
		chunkFile, err := os.CreateTemp(targetDir, "chunk_*.tmp")
		if err != nil {
			return err
		}
		defer chunkFile.Close()

		// Write range in bytes to save on RAM
		writer := bufio.NewWriter(chunkFile)
		for _, line := range compacted {
			writer.WriteString(line)
			writer.WriteByte('\n')
		}
		writer.Flush()

		// Save path of current chunk
		chunkPaths = append(chunkPaths, chunkFile.Name())
		memoryBuffer = memoryBuffer[:0] // Flush buffer saving allocated space
		return nil
	}

	// Reading downloaded files
	for _, file := range tmpFiles {
		f, err := os.Open(file)
		if err != nil {
			slog.Error("Failed to open downloaded file", "file", file, "err", err)
			// If even one file failed to open we can get bad diff data
			// So return false and threating as error
			return false
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			// If we in IP mode then searching and unpacking CIDRs on the fly
			if lists == "ip" && strings.ContainsRune(line, '/') {
				if prefix, err := netip.ParsePrefix(line); err == nil {
					addr := prefix.Addr()
					for prefix.Contains(addr) {
						// Add single IP
						memoryBuffer = append(memoryBuffer, addr.String())
						addr = addr.Next()

						if addr == (netip.Addr{}) {
							break
						}

						// In case of buffer overflow when unpacking huge subnets (/16, /8)
						if len(memoryBuffer) >= chunkSizeLimit {
							// Dump chunk on disk
							if err := dumpBufferToDisk(); err != nil {
								slog.Error("Failed to dump chunk", "err", err)
								// Close file and exit if error
								f.Close()
								return false
							}
						}
					}
					continue
				}
			}

			// Common domains, community or single IPs
			memoryBuffer = append(memoryBuffer, line)

			// If buffer overflow
			if len(memoryBuffer) >= chunkSizeLimit {
				if err := dumpBufferToDisk(); err != nil {
					slog.Error("Failed to dump chunk", "err", err)
					// Close file and exit if error
					f.Close()
					return false
				}
			}
		}
		// Close file
		f.Close()
		// Check for errors
		if err := scanner.Err(); err != nil {
			slog.Error("An error has occured in merge scanner", "file", file, "err", err)
			// Still we dont want to check diffs 
			// when part of file might be gone in case of error
			return false
		}
	}

	// Dump rest of data
	if err := dumpBufferToDisk(); err != nil {
		slog.Error("Failed to dump final chunk", "err", err)
		return false
	}

	// If no chunks (empty files)
	if len(chunkPaths) == 0 {
		os.WriteFile(finalTmpPath, []byte(""), 0644)
		return false
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
				outChunk, err := mergeTwoSortedFiles(chunkPaths[i], chunkPaths[i+1], targetDir)
				if err != nil {
					slog.Error("Error merging chunks", "err", err)
					return false
				}
				// Send new chunk to next round of merge
				nextRoundChunks = append(nextRoundChunks, outChunk)

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

	// The latest left file is our sorted and deduplicated result
	err = os.Rename(chunkPaths[0], finalTmpPath)
	if err != nil {
		slog.Error("Failed to rename final chunk", "err", err)
		return false
	}

	return true
}

// mergeTwoSortedFiles reading two files line by line and merges them into one in alphabetical order
// and deduplication. RAM consumption = memory on two lines.
func mergeTwoSortedFiles(file1, file2, targetDir string) (string, error) {
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
	outFile, err := os.CreateTemp(targetDir, "merge-*.tmp")
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	// Start two scanners
	sc1 := bufio.NewScanner(f1)
	sc2 := bufio.NewScanner(f2)
	writer := bufio.NewWriter(outFile)

	has1 := sc1.Scan()
	has2 := sc2.Scan()

	var lastWritten []byte

	// Helper to safe write of unique string
	writeFunc := func(data []byte) error {
		// Check for global duplicate (compare to latest written string)
		if len(lastWritten) == 0 || !bytes.Equal(data, lastWritten) {
			writer.Write(data)
			writer.WriteByte('\n')
			// Copy bytes so scanner can rewrite its buffer on next step
			lastWritten = append(lastWritten[:0], data...)
		}
		return nil
	}

	// Two Pointers method. Store in RAM only two lines.
	for has1 && has2 {
		b1 := sc1.Bytes()
		b2 := sc2.Bytes()

		cmp := bytes.Compare(b1, b2)
		if cmp == 0 {
			// Lines identical. Write from file 1 - scan next lines
			writeFunc(b1)
			has1 = sc1.Scan()
			has2 = sc2.Scan()
		} else if cmp < 0 {
			// Line from file 1 less then it not existing in file 2.
			// Write from file 1 - scan next line of file 1
			writeFunc(b1)
			has1 = sc1.Scan()
		} else {
			// Line from file 2 less then it not existing in file 1.
			// Write from file 2 - scan next line of file 2
			writeFunc(b2)
			has2 = sc2.Scan()
		}
	}

	// Write rest if one file ended before another
	for has1 {
		// Works only if file 2 is ended
		writeFunc(sc1.Bytes())
		has1 = sc1.Scan()
	}
	for has2 {
		// Works only if file 1 is ended
		writeFunc(sc2.Bytes())
		has2 = sc2.Scan()
	}

	// Flush data on disk from buffer 
	writer.Flush()

	// Check for errors
	if err := sc1.Err(); err != nil {
		slog.Error("An error has occured in scanner while merging temporary files", "err", err)
		return "", err
	}
	if err := sc2.Err(); err != nil {
		slog.Error("An error has occured in scanner while merging temporary files", "err", err)
		return "", err
	}

	return outFile.Name(), nil
}
