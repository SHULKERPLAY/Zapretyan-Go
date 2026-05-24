package diffscanner

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"zapretyan-go/internal/community"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/diffprocess"
	"zapretyan-go/internal/downloader"
	"zapretyan-go/internal/eventor"
	"zapretyan-go/internal/hasher"
)

// Define filenames
const newDomainFN string = "new.txt"
const oldDomainFN string = "old.txt"
const tmpDomainFN string = "new.tmp"
const newIpFN string = "newip.txt"
const oldIpFN string = "oldip.txt"
const tmpIpFN string = "newip.tmp"
const communityFN string = "community.txt"
const tmpCommunityFN string = "community.tmp"

func Handler(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done() // Report that function is ended
	defer slog.Debug("Handler() ended")

	interval := time.Duration(config.Params.ReportInterval) * time.Hour
	// Create ticker that create event every (report_interval) hours
	ticker := time.NewTicker(interval)
	defer ticker.Stop() // Clean resources on exit

	slog.Info("Started event scanner", "interval", interval)

	// First start of scan logic
	slog.Info("Scanning for new changes...")
	scan(ctx)

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
	// Run GC
	defer runtime.GC()

	// Remove temporary files after process
	defer downloader.DeleteTmpFiles(config.DataParams.DataDirectory)

	// Define Paths
	var dpath = filepath.Join(config.DataParams.DataDirectory, newDomainFN)     // Full path to domains file
	var dpatht = filepath.Join(config.DataParams.DataDirectory, tmpDomainFN)    // Full path to temporary domains file
	var dpatho = filepath.Join(config.DataParams.DataDirectory, oldDomainFN)    // Full path to old domains file
	var ipath = filepath.Join(config.DataParams.DataDirectory, newIpFN)         // Full path to IPs file
	var ipatht = filepath.Join(config.DataParams.DataDirectory, tmpIpFN)        // Full path to temporary IPs file
	var ipatho = filepath.Join(config.DataParams.DataDirectory, oldIpFN)        // Full path to old IPs file
	var cpath = filepath.Join(config.DataParams.DataDirectory, communityFN)     // Full path to community domains file
	var cpatht = filepath.Join(config.DataParams.DataDirectory, tmpCommunityFN) // Full path to temporary community domains file

	// Remove temporary files if left from last start
	downloader.DeleteTmpFiles(config.DataParams.DataDirectory)

	// Hold function for extensions to start
	hold := holdAction(ctx, &config.Params.ExtReady, 6, 10)
	if !hold {
		slog.Error("SCAN CANCELLED BY EXTENSION HANDLER OR CONTEXT")
		return
	}

	// Define updates state
	isDomain, isIp, isCommunity := defineUpdates(dpath, ipath, cpath)

	if ctx.Err() != nil {
		slog.Warn("Scanner stopped by context")
		return
	}

	// Create localWaitgroup for scan processes
	var localWg sync.WaitGroup

	// Download and merge domain lists
	domainch := make(chan bool)
	if isDomain {
		localWg.Add(1)
		go func(){
			res := community.ListDownloadAndMerge(ctx, &localWg, config.DataParams.DomainSource, config.DataParams.DataDirectory, dpatht, "domain")
			domainch <- res
		}()
	} else {
		// If goroutine not started write false into channel to not block reading
		domainch <- false 
	}

	// Download and merge ip lists
	ipch := make(chan bool)
	if isIp {
		localWg.Add(1)
		go func(){
			res := community.ListDownloadAndMerge(ctx, &localWg, config.DataParams.IpSource, config.DataParams.DataDirectory, ipatht, "ip")
			ipch <- res
		}()
	} else {
		ipch <- false 
	}

	// Download and merge community lists
	comch := make(chan bool)
	if isCommunity {
		localWg.Add(1)
		go func(){
			res := community.ListDownloadAndMerge(ctx, &localWg, config.DataParams.ComDomainSources, config.DataParams.DataDirectory, cpatht, "community")
			comch <- res
		}()
	} else {
		comch <- false 
	}

	// Wait for list tasks to end
	isDomain = <-domainch
	isIp = <-ipch
	isCommunity = <-comch
	localWg.Wait()

	if ctx.Err() != nil {
		slog.Warn("Scanner stopped by context")
		return
	}

	// Check hash to check if file rotation needed
	if config.DataParams.Method == "hash" {
		isDomain, isIp, isCommunity = hashcheck(dpath, ipath, cpath, dpatht, ipatht, cpatht, isDomain, isIp, isCommunity)
	}

	// Rotate latest updated files
	diffprocess.RotateFiles(dpath, ipath, dpatho, ipatho, dpatht, ipatht, cpath, cpatht, isDomain, isIp, isCommunity)

	// Start Diff computing
	diffs := diffprocess.CheckDiff(dpath, dpatho, ipath, ipatho, isDomain, isIp)

	// Create Core rkn Event
	eventor.CreateRknEvent(ctx, &localWg, diffs, dpath, ipath)

	slog.Info("Scan completed!")
}

// Check which lists has updates. True means need to check difference
// Also downloading lists if need to calculate diff
func defineUpdates(dpath, ipath, cpath string) (bool, bool, bool) {
	defer slog.Debug("defineUpdates() ended")

	var isDomain    bool // Is newer domains available?
	var isIp 		bool // Is newer ips available?
	var isCommunity bool // Is newer community available?

	switch config.DataParams.Method {
	case "http":
		// Check for domain list updates
		isDomain = checkUpdates(dpath, config.DataParams.DomainSource)

		// Check for ip list updates
		if !config.Params.DisableIP {
			isIp = checkUpdates(ipath, config.DataParams.IpSource)
		}

		// Check for community list updates
		if !config.Params.DisableCommunity {
			isCommunity = checkUpdates(cpath, config.DataParams.ComDomainSources)
		}
	case "hash":
		// Hashcheck need to download and merge files first.
		// Define which types we need to download and after that hashcheck will be performed
		// to decide if files were changed and if we need to rotate files

		// Domains always need to download for hashcheck
		isDomain = true
		// Is IP disabled?
		isIp = !config.Params.DisableIP
		// Is community disabled?
		isCommunity = !config.Params.DisableCommunity
	}

	// Return results
	return isDomain, isIp, isCommunity
}

// Returns true if at least one file from []sources is newer than file in localPath 
func checkUpdates(localPath string, sources []string) bool {
	defer slog.Debug("checkUpdates() ended")

	// Check if local file is exsisting
	localInfo := config.GetPathState(localPath)
	var localTime time.Time
	if localInfo.Exists {
		localTime = localInfo.ModTime
	} else {
		slog.Info("File not found and will be downloaded", "file", communityFN)
		// If file not found we need to download it
		return true
	}

	// Check remote http urls for updates
	return downloader.HasNewerRemoteFiles(localTime, sources)
}

// Check file hashes and return true (domain, ip, community) if enabled and temporary file is newer than current one.
// Specify filepaths to every needed file as string and toggle hashchecks as boolean.
// Use to check is file rotation needed for list types.
func hashcheck(newTxt, newIpTxt, communityTxt, newTmp, newIpTmp, communityTmp string, isDomain, isIp, isCommunity bool) (bool, bool, bool) {
	defer slog.Debug("hashcheck() ended")

	// Predefine states to return
	var domainState bool
	var ipState bool
	var communityState bool

	// If not false then mode enabled, download and merge of files was successful
	// If domain list enabled
	if isDomain {
		// Compare hashes
		res, err := hasher.CompareFilesHash(newTxt, newTmp)
		// Set false if hashes identical or error (Means that rotation will be ignored)
		if err != nil {
			slog.Warn("Error while comparing hashes", "hash1", newTxt, "hash2", newTmp, "err", err)
		} else {
			domainState = !res
		}
	}
	// If ip list enabled
	if isIp {
		res, err := hasher.CompareFilesHash(newIpTxt, newIpTmp)
		if err != nil {
			slog.Warn("Error while comparing hashes", "hash1", newIpTxt, "hash2", newIpTmp, "err", err)
		} else {
			ipState = !res
		}
	}
	// If community list enabled
	if isCommunity {
		res, err := hasher.CompareFilesHash(communityTxt, communityTmp)
		if err != nil {
			slog.Warn("Error while comparing hashes", "hash1", communityTxt, "hash2", communityTmp, "err", err)
		} else {
			communityState = !res
		}
	}

	return domainState, ipState, communityState
}

// Holds execution of function till core param remains false.
// Requires context, variable that should be true for continue, number of retries, interval of retry in seconds.
// Returns bool. If false: Out of retries or context closed. If true: Variable true
func holdAction(ctx context.Context, action *bool, retries int, interval int) bool {
	defer slog.Debug("holdAction() ended")
	retryAfter := time.Duration(interval)
	for i := 0; i < retries; i++ {
		condition := *action
		if condition {
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
