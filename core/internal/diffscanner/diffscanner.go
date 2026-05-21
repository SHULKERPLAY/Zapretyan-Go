package diffscanner

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"time"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/downloader"
	"zapretyan-go/internal/hasher"
)

// Define filenames
const newDomainFN = "new.txt"
const oldDomainFN = "old.txt"
const tmpDomainFN = "new.tmp"
const newIpFN = "newip.txt"
const oldIpFN = "oldip.txt"
const tmpIpFN = "newip.tmp"
const communityFN = "community.txt"
const tmpCommunityFN = "community.tmp"

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
			scan(ctx)

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

	// Define updates state
	isDomain, isIp, isCommunity := defineUpdates()

	if ctx.Err() != nil {
		slog.Warn("Scanner stopped by context")
		return
	}
	
}

// Check which lists has updates. True means need to check difference
// Also downloading lists if need to calculate diff
func defineUpdates() (bool, bool, bool) {
	defer slog.Debug("defineUpdates() ended")

	var isDomain 	bool // Is newer domains available?
	var isIp 		bool // Is newer ips available?
	var isCommunity bool // Is newer community available?

	dpath := filepath.Join(config.DataParams.DataDirectory, newDomainFN)  // Full path to domains file
	dpatht := filepath.Join(config.DataParams.DataDirectory, tmpDomainFN) // Full path to temporary domains file
	ipath := filepath.Join(config.DataParams.DataDirectory, newIpFN)	  // Full path to IPs file
	ipatht := filepath.Join(config.DataParams.DataDirectory, tmpIpFN)	  // Full path to temporary IPs file

	switch config.DataParams.Method {
	case "http":
		// Check remote http server
		isDomain = downloader.IsLocalFileOutdated(config.DataParams.DomainSource, config.DataParams.DataDirectory, newDomainFN)
		isIp = downloader.IsLocalFileOutdated(config.DataParams.IpSource, config.DataParams.DataDirectory, newIpFN)
		isCommunity = checkCommunityUpdates(config.DataParams.DataDirectory, communityFN)

		if isDomain {
			err := downloader.DownloadFile(config.DataParams.DomainSource, dpatht)
			if err != nil {
				slog.Error("Failed to GET file", "url", config.DataParams.DomainSource, "name", tmpDomainFN, "err", err)
				isDomain = false
			}
		}

		if isIp {
			err := downloader.DownloadFile(config.DataParams.IpSource, ipatht)
			if err != nil {
				slog.Error("Failed to GET file", "url", config.DataParams.IpSource, "name", tmpIpFN, "err", err)
				isIp = false
			}
		}
	case "hash":
		// Is community disabled?
		isCommunity = !config.Params.DisableCommunity 
		
		// Is domains updated?
		derr := downloader.DownloadFile(config.DataParams.DomainSource, dpatht)
		if derr != nil {
			slog.Error("Failed to GET file", "url", config.DataParams.DomainSource, "name", tmpDomainFN, "err", derr)
			isDomain = false
		} else {
			res, err := hasher.CompareFilesHash(dpath, dpatht)
			isDomain = !res
			if err != nil {
				slog.Warn("Error while comparing hashes", "hash1", dpath, "hash2", dpatht, "err", err)
			}
		}

		// Is ips updated?
		ierr := downloader.DownloadFile(config.DataParams.IpSource, ipatht)
		if ierr != nil {
			slog.Error("Failed to GET file", "url", config.DataParams.IpSource, "name", tmpIpFN, "err", ierr)
			isIp = false
		} else {
			res, err := hasher.CompareFilesHash(ipath, ipatht)
			isIp = !res
			if err != nil {
				slog.Warn("Error while comparing hashes", "hash1", ipath, "hash2", ipatht, "err", err)
			}
		}
	}

	// Return results
	return isDomain, isIp, isCommunity
}

func checkCommunityUpdates(localDir string, fileName string) bool {
	defer slog.Debug("checkCommunityUpdates() ended")
	if config.Params.DisableCommunity {
		return false
	}

	localPath := filepath.Join(localDir, fileName)

	// Check if local file is exsisting
	localInfo := config.GetPathState(localPath)
	var localTime time.Time
	if localInfo.Exists {
		localTime = localInfo.ModTime
	} else {
		slog.Info("File not found and will be downloaded", "file", communityFN)
		// If file not found set old date to redownload it
		localTime = time.Unix(0, 0)
	}

	// List of sources
	sources := config.DataParams.ComDomainSources

	// Check
	return downloader.HasNewerRemoteFiles(localTime, sources)
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
