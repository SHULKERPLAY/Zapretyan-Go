package utils

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"zapretyan-go/internal/flags"
)

// UpdateModTime force changing Last Opened and Last Modified
// properties of file to current system time.
func UpdateModTime(filePath string) error {
	slog.Debug("UpdateModTime() ended")
	// Get current system time
	currentTime := time.Now()

	// os.Chtimes Accepts:
	// Path to file (string)
	// Last Opened time (Atime)
	// Last modified time (Mtime)
	// Change both params to current time without opening file
	err := os.Chtimes(filePath, currentTime, currentTime)
	if err != nil {
		return err
	}

	return nil
}

// Define child process with path to executable. Returns *exec.Cmd
// On linux just creating executable state and returns it.
// On windows starting .exe file directly and Terminal files with CMD
func ExecuteOS(path string) *exec.Cmd {
	slog.Debug("ExecuteOS() ended")
	// Predefine variable
	var cmd *exec.Cmd

	// Execute batch files with Windows Terminal
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".bat" || ext == ".cmd" || ext == ".sh" { 
			cmd = exec.Command("cmd", "/c", path)
			return cmd
		}
	}

	// Default case
	cmd = exec.Command(path)
	return cmd
}

// Converts slashes and add .exe suffix on windows if not specified
func ExecPath(configPath string) string {
	defer slog.Debug("ExecPath() ended")
	// Convert slashes to current OS
	fixedPath := filepath.FromSlash(configPath)

	// If Windows add ".exe" to file or ignore if alredy has executable extension
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(fixedPath))

		isExec := ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".sh"

		// If it not executable file - force add .exe
		if !isExec {
			fixedPath += ".exe"
		}
	}

	return fixedPath
}

// PathState describe full state of path
type PathState struct {
	Exists       bool      // Is exist on disk?
	IsDir        bool      // Is it Directory?
	IsFile       bool      // Is it File?
	IsExecutable bool      // Can we execute it?
	AbsPath      string    // Normalized absolute Path
	ModTime      time.Time // Time when file was modified (Use with .UTC())
}

// GetPathState doing complex validation of pathstring
func GetPathState(rawPath string) PathState {
	defer slog.Debug("GetPathState() ended")
	var state PathState

	// Normalize path (Convert all ./ ../ to real path)
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		// If we cannot get abs path then syntax is broken
		return state
	}
	state.AbsPath = abs

	// Request metadata from OS
	info, err := os.Stat(abs)
	if err != nil {
		// If file not exist - return struct with Exists = false
		if os.IsNotExist(err) {
			return state
		}
		// Other errors (e.g. Access Denied) means that file exists but we cannot read it
		return state
	}

	// If all fine then fill base flags
	state.Exists = true
	state.IsDir = info.IsDir()
	state.IsFile = !info.IsDir()

	// Get last modified date
	state.ModTime = info.ModTime()

	// Check if executable (Files only)
	if state.IsFile {
		if runtime.GOOS == "windows" {
			// On Windows check file extension
			ext := strings.ToLower(filepath.Ext(abs))
			state.IsExecutable = ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".sh"
		} else {
			// On Linux/macOS check POSIX rights bits (+x for owner/group/everyone)
			state.IsExecutable = info.Mode()&0111 != 0
		}
	}

	return state
}

// Validate URL. Return true only if URL has http or https protocol
func IsValidURL(s string) bool {
	defer slog.Debug("IsValidURL() ended")
	u, err := url.ParseRequestURI(s)
	slog.Debug("URL Test", "url", s, "scheme", u.Scheme, "err", err)
	if err != nil {
		return false // String is not valid URI
	}
	// Check that proto is http(s)
	return u.Scheme == "http" || u.Scheme == "https"
}

// Universal Helper for output debug structure in console
func DumpStruct(title string, v interface{}) {
	defer slog.Debug("DumpStruct() ended")
	slog.Debug("--- [DEBUG DUMP] ---", "name", title)

	// Protection: If nil pointer output nil for debug
	if v == nil {
		slog.Debug("<nil> (Structure not initialized)")
		return
	}

	bytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		slog.Debug("ERROR formatting JSON", "err", err)
		return
	}
	slog.Debug(string(bytes))
	slog.Debug("--- [END OF DUMP] ---")
}

// Output memory info in log with specified delay in seconds.
// Boolean gc decides whether to start GC before delay.
// gcDelay int specifies delay before starting GC. gcDelay also increases memory info log output delay.
// It is recomended to start from goroutine. It is recomended to start GC ONLY IF ALL ACTIVE OPERATIONS WAS COMPLETED!
func DumpMemoryStatistics(delay int, gc bool, gcDelay int) {
	if flags.Args.Loglevel != "debug" { // DEBUG ONLY FUNCTION
		return
	} 
	// Start GC?
	if gc {
		// Delay to start GC
		if gcDelay >= 1 {
			time.Sleep(time.Duration(gcDelay) * time.Second) // Sleep n Seconds before start GC
		}
		runtime.GC()
	}
	// Delay of data after GC step
	if delay >= 1 {
		time.Sleep(time.Duration(delay) * time.Second) // Sleep n Seconds before output statistics
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	slog.Debug("--- [MEMSTAT] ---")
	slog.Debug("Live memory on heap", "(KB)", m.Alloc/1024)
	slog.Debug("Total handling by GO runtime", "(KB)", m.Sys/1024)
	slog.Debug("Allocated from total for goroutine stacks", "(KB)", m.StackInuse/1024)
	slog.Debug("Cleaned by GC, but not returned for OS", "(KB)", m.HeapReleased/1024)
	slog.Debug("--- [END OF MEMSTAT] ---")
}

// Helper to stop execution until user press enter.
// Created for users to catch up with logs in ui while app is closing.
// e.g. on windows if core not started from cmd window will quickly close on app exit
// and user do not catch with log reading.
// Pauses are ignored in system service mode!
func Pause() {
	if !flags.Args.Service {
		slog.Warn("Press enter to continue...")
		// Read one byte from stdin (this is enough to block the flow)
		var b [1]byte
		os.Stdin.Read(b[:])
	}
}
