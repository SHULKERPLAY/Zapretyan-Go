package downloader

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"zapretyan-go/internal/config"
)

// HasNewerRemoteFiles checks URL array and returns true,
// if at least one server has newer file than baseTime.
func HasNewerRemoteFiles(baseTime time.Time, urls []string) bool {
	defer slog.Debug("HasNewerRemoteFiles() ended")

	if len(urls) == 0 {
		return false
	}

	// Context with cansel need to interrupt all requests
	// when one of servers returns true (reduce network and time consumption)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel for collecting results from goroutines
	resultChan := make(chan bool, len(urls))
	var wg sync.WaitGroup

	// Start check for every link at once
	for _, url := range urls {
		wg.Add(1)
		go func(fileURL string) {
			defer wg.Done()

			// Send quick HEAD request on server to get headers
			// With cancel support through context
			req, err := http.NewRequestWithContext(ctx, "HEAD", fileURL, nil)
			if err != nil {
				slog.Warn("Something went wrong...", "url", fileURL, "err", err)
				return
			}

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				// Not breaking all other checks if one server is down
				slog.Warn("Error while trying to get server headers", "url", fileURL, "err", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				slog.Warn("Server returned wrong HTTP status!", "status", resp.Status, "url", fileURL)
				return
			}

			// Get Last-Modified tag from headers reply
			remoteLastModifiedStr := resp.Header.Get("Last-Modified")
			if remoteLastModifiedStr == "" {
				// If server not return date, safer to download it and check
				slog.Warn("Server does not returned Last-Modified header. Core can download file to check for changes", "url", fileURL)
				resultChan <- true
				cancel() // Cancel all other requests
				return
			}

			remoteTime, err := time.Parse(time.RFC1123, remoteLastModifiedStr)
			if err != nil {
				slog.Warn("Error while parse server date", "date_str", remoteLastModifiedStr, "err", err)
				return
			}

			// If file on server is newer than our base date
			if remoteTime.UTC().After(baseTime.UTC()) {
				slog.Info("Found community update", "url", fileURL, "remote", remoteTime.Format(time.RFC3339))
				resultChan <- true
				cancel() // Found? Cancel all other requests
				return
			}
		}(url)
	}

	// Goroutine to close the channel when all check is done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Wait for result. If channel accepts at least one true - exit.
	// If channel closed without results (All false or failed), return false.
	for hasNew := range resultChan {
		if hasNew {
			return true
		}
	}

	return false
}

// isLocalFileOutdated checks if local file is outdated comparing to remote server.
// Return true if file has updates or false if file on remote server same or older.
// Requires URL to remote file, Local directory of compared file and local file name.
func IsLocalFileOutdated(fileURL string, localDir string, fileName string) bool {
	defer slog.Debug("isLocalFileOutdated() ended")

	localPath := filepath.Join(localDir, fileName)

	// Check if local file is exsisting
	localInfo := config.GetPathState(localPath)
	if !localInfo.Exists {
		slog.Info("Local file is not found. Downloading required", "file", fileName)
		return true // If file not exist then it is outdated
	}

	// Send quick HEAD request on server to get headers
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Head(fileURL)
	if err != nil {
		slog.Error("Error while trying to get server headers", "url", fileURL, "err", err)
		return false // Stay with local file if server is unavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("Server returned wrong HTTP status!", "status", resp.Status, "url", fileURL)
		return false
	}

	// Get Last-Modified tag from headers reply
	remoteLastModifiedStr := resp.Header.Get("Last-Modified")
	if remoteLastModifiedStr == "" {
		slog.Warn("Server does not returned Last-Modified header. Core can download file to check for changes", "file", fileName)
		return true // If server not return date, safer to download it and check
	}

	// Parse server date. (HTTP-date always in RFC1123 format)
	remoteTime, err := time.Parse(time.RFC1123, remoteLastModifiedStr)
	if err != nil {
		slog.Error("Error while parse server date", "date_str", remoteLastModifiedStr, "err", err)
		return false
	}

	// Compare last modified dates (round local time to seconds cuz HTTP does not stores ms)
	localTime := localInfo.ModTime.UTC()

	if remoteTime.After(localTime) {
		slog.Info("Found update on remote server",
			"file", fileName,
			"local", localTime.Format(time.RFC3339),
			"remote", remoteTime.Format(time.RFC3339),
		)
		return true // File on server is newer
	}

	slog.Debug("No updates found for local file", "file", fileName)
	return false // File on server the same or older
}

// Downloads file from URL and store it in specified destPath
func DownloadFile(url string, destPath string) error {
	defer slog.Debug("DownloadFile() ended")

	// Automaticly create folders if they not exist
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	slog.Info("Started download", "from", url)

	// Send GET request to server
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create file on disk
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Stream write data from network to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	slog.Info("File downloaded", "path", destPath)
	return nil
}

// DeleteTmpFiles removes all files with .tmp suffix in directory dirPath.
func DeleteTmpFiles(dirPath string) {
	// Read only content of current directory
	files, err := os.ReadDir(dirPath)
	if err != nil {
		slog.Error("[.tmp REMOVER] Error reading directory contents", "err", err)
		return
	}

	for _, file := range files {
		// Check if object is file and he has .tmp suffix
		if !file.IsDir() && filepath.Ext(file.Name()) == ".tmp" {
			fullPath := filepath.Join(dirPath, file.Name())
			if err := os.Remove(fullPath); err != nil {
				slog.Error("Error removing temporary file", "err", err)
				return
			}
		}
	}
}