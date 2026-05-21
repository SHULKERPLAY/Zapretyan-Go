package diffscanner

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"zapretyan-go/internal/config"
)

func Handler(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done() // Report that function is ended
	defer slog.Debug("Handler() ended")

	interval := time.Duration(config.Params.ReportInterval) * time.Hour
	// Create ticker that create event every (report_interval) hours
	ticker := time.NewTicker(interval)
	defer ticker.Stop() // Clean resources on exit

	slog.Info("Started event scanner for every", "hours", interval)

	for {
		select {
		case <-ticker.C:
			// Scan logic
			slog.Info("Scanning for new changes...")
			// performScan()

		case <-ctx.Done():
			// Graceful shutdown
			slog.Info("Stopping event scanner...")
			return
		}
	}
}

func scan(ctx context.Context) {
	defer slog.Debug("scan() ended")

	// Hold function for extensions to start
	hold := holdAction(ctx, config.Params.ExtReady, 6, 10)
	if !hold {
		slog.Error("SCAN CANCELLED BY EXTENSION HANDLER OR CONTEXT")
		return
	}

	switch config.DataParams.Method {
	case "http":
		//
	case "hash":
		//
	}
}

// isLocalFileOutdated checks if local file is outdated comparing to remote server.
// Return true if file has updates or false if file on remote server same or older.
// Requires URL to remote file, Local directory of compared file and local file name.
func isLocalFileOutdated(fileURL string, localDir string, fileName string) bool {
	defer slog.Debug("isLocalFileOutdated() ended")

	localPath := filepath.Join(localDir, fileName)

	// Check if local file is exsisting
	localInfo := config.GetPathState(localPath)
	if !localInfo.Exists {
		slog.Info("Local file is not found. Downloading required", "file", fileName)
		return true // If file not exist then it is outdated
	}
	localStat, _ := os.Stat(localPath)

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
	localTime := localStat.ModTime().UTC()

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

// Holds execution of function till core param remains false.
// Requires context, variable that should be true for continue, number of retries, interval of retry in seconds.
// Returns bool. If false: Out of retries or context closed. If true: Variable true
func holdAction(ctx context.Context, action bool, retries int, interval int) bool {
	defer slog.Debug("holdAction() ended")
	retryAfter := time.Duration(interval)
	for i := 0; i < retries; i++ {
		if action {
			return true
		}

		select {
		case <-ctx.Done(): // Return immidiately if context closed
			return false
		case <-time.After(retryAfter * time.Second):
			// Continue cycle
		}
	}
	return false
}
