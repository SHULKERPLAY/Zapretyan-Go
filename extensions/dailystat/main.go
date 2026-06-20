package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Handshake is a structure of first reply to core
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

// PluginConfig - Part of structure that stores in "cfg"
type PluginConfig struct {
	IsJSON    bool          `json:"latestjson"` // Enable JSON with daily data
	IsCSV     bool       `json:"analytics"`  // Enable historical daily stats in CSV table
	JsonPath  string       `json:"json_file"`  // Path to JSON with daily data
	CsvPath   string       `json:"csv_file"`   // Path to daily stats in CSV table
	StartHour int          `json:"day_start"`  // Hour after which daily statistics will be written and daily counters resetted
	Locale    LocaleObject `json:"locale"`     // Locales
	Jsontmp   string       // JSON tmp file
}

type LocaleObject struct {
	Hrs24 string `json:"for24hrs"`
}

// BaseEvent - Base structure to understand what delivered without full parse
type BaseEvent struct {
	Ver  int           `json:"ver"`  // Core JSON format version
	Type string        `json:"type"` // "cmd" for commands and config send, "rkn" for events
	Kill bool          `json:"kill"` // If true process must exit immidiately
	Path string        `json:"path"` // Absolute path to Data directory
	Diff *DiffObject   `json:"diff"` // Diff container. If empty it will be nil
	Cfg  *PluginConfig `json:"cfg"`  // Field can be empty so use the pointer. If empty it will be nil
}

// Structure for every object in "diff"
type DiffObject struct {
	BannedDomains   DiffObjectData `json:"banned"`      // Banned domains
	UnbannedDomains DiffObjectData `json:"unbanned"`    // Unbanned domains
	BannedIPs       DiffObjectData `json:"banned_ip"`   // Banned IP
	UnbannedIPs     DiffObjectData `json:"unbanned_ip"` // Unbanned IP
}

// Structure for data objects inside "diff"
type DiffObjectData struct {
	Empty  bool `json:"empty"`  // If true then Data empty
	Length int  `json:"length"` // Length of Data array
	// Data    []string `json:"data"`// We do not need data here
	Total int `json:"total"` // Total count of type
}

// Plugin internal specs
const (
	pmode       string = "ONCE"
	pjver       int    = 1 // Expected JSON version
	pver        string = "1.0.1@f61tsi7f"
	jsontmp     string = "./statistics/stats_temp.json" // Path to temporary JSON counters
	csvDefault  string = "./statistics/analyticsV2.csv"
	jsonDefault string = "./statistics/latest.json"
)

var cfg *PluginConfig // Define config
var loc *LocaleObject // Define pointer to locales

func main() {
	var wg sync.WaitGroup
	// Send Handshake
	sendHandhake()

	// Start core events listener
	wg.Add(1)
	go coreListener(&wg)

	wg.Wait() // Wait for goroutine to end
}

// logMsg writes logs as plain text into Stderr without any prefixes
func logMsg(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// sendHandhake sending validation data to core
func sendHandhake() {
	// Object to send handshake into stdout on start
	handshake := Handshake{
		Mode:    pmode,
		Version: pver,
		JsonVer: pjver,
	}

	// Send onelined JSON with \n at the end
	if err := json.NewEncoder(os.Stdout).Encode(handshake); err != nil {
		logMsg("FATAL: Error sending Handshake (%v)", err)
		os.Exit(1)
	}
}

// coreListener hanles stdin data and send it to data processor
func coreListener(wg *sync.WaitGroup) {
	defer wg.Done() // Send to main goroutine sign that listener has stopped on return

	// Using here scanner instead of json.Decoder to control buffer size
	scanner := bufio.NewScanner(os.Stdin)

	// Allocate start buffer (e.g. 64KB) and set max buffer limit (e.g. 100MB)
	// This protect us from infinite RAM allocation on bad data
	const maxCapacity = 100 * 1024 * 1024 // 100MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		// scanner.Bytes() return pointer into internal scanner buffer.
		// No new memory is allocated on this line!
		rawBytes := scanner.Bytes()
		if len(rawBytes) == 0 {
			continue
		}

		// Convert into json.RawMessage type
		rawEvent := json.RawMessage(rawBytes)

		// Send RAW JSON data into your event processor
		handleEvent(rawEvent)
	}

	// Check why cycle is ended
	if err := scanner.Err(); err != nil {
		logMsg("Error reading from stdin: %v", err)
	} else {
		// If scanner passes till this moment without errors then data flow is closed (EOF)
		logMsg("stdin closed by core. Shutdown.")
		return
	}
}

// handleEvent decompiles event and forward data for processing
func handleEvent(raw json.RawMessage) {
	var base BaseEvent
	if err := json.Unmarshal(raw, &base); err != nil {
		logMsg("Error parsing base structure: %v", err)
		return
	}

	// Exit if "kill": true
	if base.Kill {
		logMsg("Got KILL signal. Shutdown.")
		os.Exit(0)
	}

	// Check proto ver
	if base.Ver != pjver && base.Ver != 0 {
		logMsg("Warning: Event version (%d) does not match (%d)", base.Ver, pjver)
	}

	switch base.Type {
	case "cmd":
		// If config is still empty, and JSON has "cfg" then initialize config
		if cfg == nil && base.Cfg != nil {
			// Write data from JSON
			cfg = base.Cfg
			// Process config data
			loadConfig(base.Path)

			// We can safely for RAM pass without pointers next types:
			// - String (As it just pass only 16 bytes string header)
			// - []Array (As it pass only 24 bytes of internal header)
			// - var that already a *Pointer
			// (e.g. cfg is already the pointer on *PluginConfig so we can just pass it (8 bytes))

			// We cannot pass entire base struct as it not pointer and if it will pass
			// then we clone it with full size in RAM. Structs such as base (which is NOT pointer)
			// we need to pass as address. (e.g. func func1(*BaseEvent) and call it as func1(&base))
		}

	case "rkn": // Event with possible diffs to process
		processRknEvent(base.Diff)

		if pmode == "ONCE" {
			os.Exit(1)
		}

	default:
		logMsg("Unknown event type: %s", base.Type)
	}
}

// Load and process all config data
func loadConfig(dataPath string) {
	// Check if we cannot parse path of data dir
	if dataPath == "" {
		logMsg("Failed to read core data dir. Defaulting to './data'")
		dataPath = "data"
	}

	// Load path to CSV
	if cfg.CsvPath != "" {
		cfg.CsvPath = smartJoin(dataPath, cfg.CsvPath)
	} else {
		cfg.CsvPath = smartJoin(dataPath, csvDefault)
		logMsg("Failed to read 'csv_file'. Defaulting to '%s'", cfg.CsvPath)
	}

	// Load path to JSON
	if cfg.JsonPath != "" {
		cfg.JsonPath = smartJoin(dataPath, cfg.JsonPath)
	} else {
		cfg.JsonPath = smartJoin(dataPath, jsonDefault)
		logMsg("Failed to read 'csv_file'. Defaulting to '%s'", cfg.JsonPath)
	}

	// Build path to temporary JSON
	cfg.Jsontmp = smartJoin(dataPath, jsontmp)

	// Check hour integer
	if cfg.StartHour < 0 || cfg.StartHour > 23 {
		cfg.StartHour = 0
		logMsg("Bad 'day_start'. Defaulting to %v", cfg.StartHour)
	}

	// Check locales
	loc = &cfg.Locale
	loc.Hrs24 = validateString(loc.Hrs24, "за 24 часа")

	// Automaticly create folders if they not exist
	if err := os.MkdirAll(filepath.Dir(cfg.CsvPath), 0755); err != nil {
		logMsg("Error creating directory '%v': %v", cfg.CsvPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.JsonPath), 0755); err != nil {
		logMsg("Error creating directory '%v': %v", cfg.JsonPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Jsontmp), 0755); err != nil {
		logMsg("Error creating directory '%v': %v", cfg.Jsontmp, err)
	}

	logMsg("Config loaded")
	logMsg("JSON: %v, CSV: %v, Hour: %v", cfg.IsJSON, cfg.IsCSV, cfg.StartHour)
	logMsg("JSON Path: %v", cfg.JsonPath)
	logMsg("CSV Path: %v", cfg.CsvPath)
}

// Check if string empty and return fallback if it is
func validateString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return strings.TrimSpace(value)
}

// Return absolute path if target is absolute.
// If path is relative returns joined path string
func smartJoin(base, target string) string {
	// If target absolute return it without joining
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	// Если относительный — безопасно склеиваем
	return filepath.Join(base, target)
}

// processRknEvent
func processRknEvent(diff *DiffObject) {
	// Check if config persists
	if cfg == nil {
		logMsg("FATAL: Log pointer is nil")
		return
	}

	// Process data
	if err := UpdateDailyStats(
		diff.BannedDomains.Length,
		diff.UnbannedDomains.Length,
		diff.BannedIPs.Length,
		diff.UnbannedIPs.Length,
		diff.BannedDomains.Total,
		diff.BannedIPs.Total,
	); err != nil {
		logMsg("Error while updating daily statistics:")
	} else {
		logMsg("JSON Counters updated")
	}
}

// DailyStats is a JSON structure.
// The ",string" tag forces to write integers as "string" ("1")
type DailyStats struct {
	TodayBan      int    `json:"todayban,string"`
	TodayUnban    int    `json:"todayunban,string"`
	TotalBanStr   string `json:"totalban"`
	RawTotalBan   int    `json:"rawtotalban,string"`
	TodayIPBan    int    `json:"todayipban,string"`
	TodayIPUnban  int    `json:"todayipunban,string"`
	TotalIPBanStr string `json:"totalipban"`
	RawTotalIPBan int    `json:"rawtotalipban,string"`
}

// getLogicalDay return date with preserving rolloverHour.
// If current time less than rolloverHour then count it as past day.
func getLogicalDay(t time.Time, rolloverHour int) time.Time {
	year, month, day := t.Date()
	logicalDay := time.Date(year, month, day, 0, 0, 0, 0, t.Location())

	if t.Hour() < rolloverHour {
		logicalDay = logicalDay.AddDate(0, 0, -1)
	}
	return logicalDay
}

// UpdateDailyStats calls every time when new data appears.
func UpdateDailyStats(deltaBan, deltaUnban, deltaIpBan, deltaIpUnban, currentTotalBan, currentTotalIpBan int) error {
	now := time.Now()

	// Check if new day has started (rotation)
	tempStat, err := os.Stat(cfg.Jsontmp)
	if err == nil {
		tempLogicalDay := getLogicalDay(tempStat.ModTime(), cfg.StartHour)
		nowLogicalDay := getLogicalDay(now, cfg.StartHour)

		// If logical day of file less than current day then it is new day
		if tempLogicalDay.Before(nowLogicalDay) {
			// Write data into CSV
			if err := LogToCSV(cfg.CsvPath); err != nil {
				logMsg("ERROR: While writing CSV: %v", err)
			} else {
				logMsg("CSV data Updated")
			}

			// Rename temporary file to main.
			// It ends past day counters and replaces old JSON with current temporary.
			_ = os.Rename(cfg.Jsontmp, cfg.JsonPath)
			logMsg("JSON Rotated")
		}
	}

	// Read of main (old) file 
	// We need its data to get diff of total counters within a day.
	var mainStats DailyStats
	mainData, err := os.ReadFile(cfg.JsonPath)
	if err == nil {
		_ = json.Unmarshal(mainData, &mainStats)
	} else {
		// If main file not exist (First launch)
		// take curent totals as base value for diff value was +0 and not +1447153
		mainStats.RawTotalBan = currentTotalBan
		mainStats.RawTotalIPBan = currentTotalIpBan
	}

	// Read current temporary JSON
	var tempStats DailyStats
	tempData, err := os.ReadFile(cfg.Jsontmp)
	if err == nil {
		_ = json.Unmarshal(tempData, &tempStats)
	}
	// If file not found (Moved while rotating or first launch)
	// tempStats will be an empty struct with zeros for new counters!

	// Data updating
	// Add new events to accumulated for today
	tempStats.TodayBan += deltaBan
	tempStats.TodayUnban += deltaUnban
	tempStats.TodayIPBan += deltaIpBan
	tempStats.TodayIPUnban += deltaIpUnban

	// Replace base values with gathered data
	tempStats.RawTotalBan = currentTotalBan
	tempStats.RawTotalIPBan = currentTotalIpBan

	// Count diff of now and the end of past day (main file)
	diffBan := tempStats.RawTotalBan - mainStats.RawTotalBan
	diffIpBan := tempStats.RawTotalIPBan - mainStats.RawTotalIPBan

	// Format strings (%+d automaticly place + prefix to positive and - prefix to negative integer)
	tempStats.TotalBanStr = fmt.Sprintf("%d `(%+d %v)`", tempStats.RawTotalBan, diffBan, loc.Hrs24)
	tempStats.TotalIPBanStr = fmt.Sprintf("%d `(%+d %v)`", tempStats.RawTotalIPBan, diffIpBan, loc.Hrs24)

	// Save back to temporary JSON
	newData, err := json.Marshal(tempStats)
	if err != nil {
		return fmt.Errorf("error while generating JSON: %w", err)
	}

	err = os.WriteFile(cfg.Jsontmp, newData, 0644)
	if err != nil {
		return fmt.Errorf("error writing temporary file: %w", err)
	}

	return nil
}

// LogToCSV Gets filepath and append historical CSV data from current temporary JSON
func LogToCSV(csvPath string) error {
	// Read current JSON tempfile
	var tempStats DailyStats
	tempData, err := os.ReadFile(cfg.Jsontmp)
	if err == nil {
		_ = json.Unmarshal(tempData, &tempStats)
	}

	// Get current date as DD.MM.YYYY
	currentDate := time.Now().Format("02.01.2006")

	// Patterns for header and newline
	header := []string{"Date", "Banned Domains", "Unbanned Domains", "Total banned Domains", "Banned IPs", "Unbanned IPs", "Total banned IPs"}
	newRow := []string{
		currentDate,
		fmt.Sprintf("%d", tempStats.TodayBan),
		fmt.Sprintf("%d", tempStats.TodayUnban),
		fmt.Sprintf("%d", tempStats.RawTotalBan),
		fmt.Sprintf("%d", tempStats.TodayIPBan),
		fmt.Sprintf("%d", tempStats.TodayIPUnban),
		fmt.Sprintf("%d", tempStats.RawTotalIPBan),
	}

	var records [][]string
	fileExisted := true

	// Read existing file (If exist)
	file, err := os.OpenFile(csvPath, os.O_RDONLY, 0644)
	if os.IsNotExist(err) {
		fileExisted = false
	} else if err != nil {
		return fmt.Errorf("не удалось открыть CSV для чтения: %w", err)
	} else {
		// File exist - read records to memory
		reader := csv.NewReader(file)
		reader.Comma = ';' // Set separator ";"

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				file.Close()
				return fmt.Errorf("ошибка парсинга CSV: %w", err)
			}
			records = append(records, record)
		}
		file.Close()
	}

	// Process data in memory
	if !fileExisted || len(records) == 0 {
		// If file not found or it is empty create struct with header and newline
		records = [][]string{header, newRow}
	} else {
		// If file exist find if line with current date already exist
		dateFound := false
		for i, record := range records {
			if len(record) > 0 && record[0] == currentDate {
				// Found current day - rewrite it with actual data
				records[i] = newRow
				dateFound = true
				break
			}
		}
		// If it is new day just add new line at the end of array
		if !dateFound {
			records = append(records, newRow)
		}
	}

	// Write data back to file (Rewrite)
	// Use O_TRUNC to fully update file with actual strings array
	outFile, err := os.OpenFile(csvPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("не удалось открыть CSV для записи: %w", err)
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	writer.Comma = ';' // Set separator to write

	// Dump array back to disk
	err = writer.WriteAll(records)
	if err != nil {
		return fmt.Errorf("ошибка записи данных в CSV: %w", err)
	}

	return nil
}
