package community

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/diffprocess"
	"zapretyan-go/internal/downloader"
)

// CommunityDownloadAndMerge downloading all files from array and merge them with deduplication, sorts and renaming.
func CommunityDownloadAndMerge(ctx context.Context, wg *sync.WaitGroup, urls []string, targetDir, finalTmpPath, finalTxtPath string) {
	defer slog.Debug("CommunityDownloadAndMerge() ended")
	defer wg.Done() // Report that function is ended

	// Create context that cancel by timer or by global context end
	localCtx, cancel := context.WithTimeout(ctx, time.Duration(config.Params.ExtOnceCtxTimeout-10)*time.Second)
	defer cancel() // Clean resources

	var localWg sync.WaitGroup
	// Channel for collecting errors data from goroutines
	errChan := make(chan error, len(urls))

	// List of paths to temporary downloaded files
	tmpFiles := make([]string, len(urls))

	// Parallel download
	for i, url := range urls {
		localWg.Add(1)
		tmpFiles[i] = filepath.Join(targetDir, fmt.Sprintf("community%d.tmp", i))

		go func(index int, downloadURL string, destPath string) {
			defer localWg.Done()

			// Pass conext to download function
			if err := downloader.DownloadFile(localCtx, downloadURL, destPath); err != nil {
				errChan <- fmt.Errorf("Error downloading %s: %w", downloadURL, err)
			}
		}(i, url, tmpFiles[i])
	}

	// Wait for goroutines to end
	localWg.Wait()
	close(errChan)

	// Check if errors while downloading
	if len(errChan) > 0 {
		slog.Error("Errors while downloading community lists", "err", errChan)
		return
	}

	// Merge, deduplicate and sort lines
	linesMap := make(map[string]struct{})

	for _, file := range tmpFiles {
		if err := readLinesToMap(file, linesMap); err != nil {
			slog.Error("Error reading community tmp file", "file", file, "err", err)
			return
		}
	}

	// Transfer unique lines into slice to sort
	uniqueLines := make([]string, 0, len(linesMap))
	for line := range linesMap {
		uniqueLines = append(uniqueLines, line)
	}
	sort.Strings(uniqueLines)

	// Write to final files
	// Write community.tmp
	if err := writeLinesToFile(finalTmpPath, uniqueLines); err != nil {
		slog.Error("Error community writing", "file", finalTmpPath, "err", err)
		return
	}

	// Quick rename community.tmp into community.txt (Old community.txt will be owerwriten)
	if err := diffprocess.RenameIfExists(finalTmpPath, finalTxtPath); err != nil {
		slog.Error("Error renaming community tmp file", "err", err)
		return
	}
}

// Reading file lines to map for deduplication
func readLinesToMap(path string, linesMap map[string]struct{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			linesMap[line] = struct{}{}
		}
	}
	return scanner.Err()
}

// Write slice of lines to file
func writeLinesToFile(path string, lines []string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return writer.Flush()
}
