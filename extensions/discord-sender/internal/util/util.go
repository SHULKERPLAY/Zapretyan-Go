package util

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"
)

// logMsg writes logs as plain text into Stderr without any prefixes
func LogMsg(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// Holds execution of function till core param remains false.
// Requires context, variable that should be true for continue, number of retries, interval of retry in seconds.
// Returns bool. If false: Out of retries or context closed. If true: Variable true
func HoldAction(ctx context.Context, action *bool, retries int, interval int) bool {
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

// Channel marker for force exiting stdin scanner. 
// When we closing it with close(StopScannerChan), all read functions exit quickly.
var StopScannerChan = make(chan struct{})

// Trigger to send close signal from any module
// to stop event scanner and cause context cancel.
func StopStdinScanner() {
	select {
	case <-StopScannerChan:
		// Already closed. Doing nothing
	default:
		close(StopScannerChan)
		os.Stdin.Close()
	}
}

// Check if string empty and return fallback if it is
func ValidateString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return strings.TrimSpace(value)
}

// Check if string exceeds min or max character limit and return fallback if it is
func ValidateLength(value, fallback string, minimum, maximum int) string {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	if length < minimum || length > maximum {
		LogMsg("Length of string '%s' exceeds range %d-%d ", value, minimum, maximum)
		return fallback
	}

	return strings.TrimSpace(value)
}

// Validate URL. Return true only if URL has http or https protocol
func IsValidURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		LogMsg("Invalid url '%s'", s)
		return false // String is not valid URI
	}
	// Check that proto is http(s)
	return u.Scheme == "http" || u.Scheme == "https"
}

// ParseHexColor accepts string ("ff5e5e", "FF5E5E" or "#ff5e5e") 
// and returns color int if parsed. If not, return default color.
func ParseHexColor(hexStr string, defaultHex int) int {
	// Clear string from spaces and '#'
	hexStr = strings.TrimSpace(hexStr)
	hexStr = strings.TrimPrefix(hexStr, "#")

	// Validate Hex color length
	if len(hexStr) != 6 {
		LogMsg("BAD HEX COLOR. Length %d instead of 6", len(hexStr))
		return defaultHex
	}

	// Convert string to integer
	// base = 16 (hex)
	// bitSize = 32 (parse in 32-bit integer)
	colorUint, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		LogMsg("FAILED TO PARSE HEX COLOR: %w", err)
		return defaultHex
	}

	// Return type of int
	return int(colorUint)
}

// Parse string and return snowflakeID. Return true if successful.
func ParseSnowflake(value string) (snowflake.ID, bool) {
	sflake, err := snowflake.Parse(value)
	if err != nil {
		LogMsg("Failed to parse SnowflakeID '%s'", value)
		return 0, false
	}
	return sflake, true
}

// Return absolute path if target is absolute.
// If path is relative returns joined path string
func SmartJoin(base, target string) string {
	// If target absolute return it without joining
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	// Если относительный — безопасно склеиваем
	return filepath.Join(base, target)
}