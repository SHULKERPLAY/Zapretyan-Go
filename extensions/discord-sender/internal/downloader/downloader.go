package downloader

import (
	"context"
	"discord-sender/internal/util"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Downloads file from URL and store it in specified destPath with retries
func DownloadFile(ctx context.Context, url string, destPath string) error {
	// Automaticly create folders if they not exist
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	const maxAttempts = 3 		  // Max attempts
	retryDelay := 5 * time.Second // Wait before retry
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check context on every retry
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt > 1 {
			util.LogMsg("Retrying to download file %d/%d...", attempt, maxAttempts)
			
			// Wait before next retry with context support
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
				// Raise wait time for next retry
				retryDelay *= 2 
			}
		}

		util.LogMsg("Started download (%s)", url)

		// Call attempt function
		lastErr = doDownloadAttempt(ctx, url, destPath)
		if lastErr == nil {
			util.LogMsg("File downloaded successfully")
			return nil
		}

		util.LogMsg("Error trying to download '%s': %v", url, lastErr)
		
		// Clear corrupted file to restart
		_ = os.Remove(destPath)
	}

	return fmt.Errorf("Failed to download file from '%v' after %d attempts: %w", url, maxAttempts, lastErr)
}

// Helper for download attempt
func doDownloadAttempt(ctx context.Context, url string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	// Send request to server
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check HTTP Status. If error 400 (except 429) do not retry
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wrong reply from server: %s", resp.Status)
	}

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

	return nil
}