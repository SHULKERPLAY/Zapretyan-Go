package events

import (
	"bufio"
	"bytes"
	"context"
	"discord-sender/internal/cfg"
	"discord-sender/internal/disclient"
	"discord-sender/internal/dissender"
	"discord-sender/internal/geomanager"
	"discord-sender/internal/util"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// Handshake is a structure of first reply to core
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

// BaseEvent - Base structure to understand what delivered without full parse
type BaseEvent struct {
	Ver  int               `json:"ver"`  // Core JSON format version
	Type string            `json:"type"` // "cmd" for commands and config send, "rkn" for events
	Kill bool              `json:"kill"` // If true process must exit immidiately
	Path string            `json:"path"` // Absolute path to Data directory
	Diff *DiffObject       `json:"diff"` // Diff container. If empty it will be nil
	Cfg  *cfg.PluginConfig `json:"cfg"`  // Field can be empty so use the pointer. If empty it will be nil
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
	Empty  bool     `json:"empty"`  // If true then Data empty
	Length int      `json:"length"` // Length of Data array
	Data   []string `json:"data"`   // All changes
	Total  int      `json:"total"`  // Total count of type
}

// coreListener hanles stdin data and send it to data processor
func CoreListener(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done() // Send to main goroutine sign that listener has stopped on return

	// Using here scanner instead of json.Decoder to control buffer size
	scanner := bufio.NewScanner(os.Stdin)

	// Allocate start buffer (e.g. 64KB) and set max buffer limit (e.g. 100MB)
	// This protect us from infinite RAM allocation on bad data
	const maxCapacity = 100 * 1024 * 1024 // 100MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	// Create buffered Channel for Events with (4) capacity
	eventQueue := make(chan json.RawMessage, 4)

	// Start event workers (2)
	for w := 1; w <= 2; w++ {
		go worker(eventQueue, ctx)
	}

	// Send Handshake
	sendHandhake()

	// Channel for async result of one iteration of Scan()
	scanResult := make(chan bool, 1)

	for {
		// Start Async scanner
		go func() {
			scanResult <- scanner.Scan()
		}()

		// Check 1: If chan closed before Scan()
		select {
		case <-util.StopScannerChan: // If scanner must stop
			util.LogMsg("Scanner stop requested before scan. Exiting.")
			goto exitSequence
		case hasNext := <-scanResult: // If scanner found line
			if !hasNext { // Check if we have next line
				goto exitSequence // If not then it is EOF
			}
		}

		// scanner.Bytes() return pointer into internal scanner buffer.
		// No new memory is allocated on this line!
		rawBytes := scanner.Bytes()
		rawBytes = bytes.TrimSpace(rawBytes)
		if len(rawBytes) == 0 {
			continue
		}

		// COPY BYTES before sending them to channel!
		cleanBytes := make([]byte, len(rawBytes))
		copy(cleanBytes, rawBytes)

		// Send to queue. If queue overflowed scanner automaticly
		// pause reading from stdin.
		eventQueue <- json.RawMessage(cleanBytes)
	}
	
	exitSequence: // To force leave the loop
	// Check why cycle is ended
	if err := scanner.Err(); err != nil {
		util.LogMsg("Error reading from stdin: %v", err)
	} else {
		// If scanner passes till this moment without errors then data flow is closed (EOF)
		util.LogMsg("stdin closed by core. Shutdown.")
	}

	// Cancelling global context
	close(eventQueue) // Close channel after scanner shutdown
	cancel()
	time.Sleep(2000 * time.Millisecond)
}

// Worker listening event channel and running long tasks to process event
func worker(events <-chan json.RawMessage, ctx context.Context) {
	// When channel is closed this func will end
	for event := range events {
		handleEvent(event, ctx) // Process in background
	}
}

// sendHandhake sending validation data to core
func sendHandhake() {
	// Object to send handshake into stdout on start
	handshake := Handshake{
		Mode:    cfg.Build.Mode,
		Version: cfg.Build.Ver,
		JsonVer: cfg.Build.JsonVer,
	}

	// Send onelined JSON with \n at the end
	if err := json.NewEncoder(os.Stdout).Encode(handshake); err != nil {
		util.LogMsg("FATAL: Error sending Handshake (%v)", err)
		os.Exit(1)
	}
}

// handleEvent decompiles event and forward data for processing.
// This func is a worker started on background and catching requests from Chan
func handleEvent(raw json.RawMessage, ctx context.Context) {
	var base BaseEvent
	if err := json.Unmarshal(raw, &base); err != nil {
		util.LogMsg("Error parsing base structure: %v", err)
		return
	}

	// Exit if "kill": true
	if base.Kill {
		util.LogMsg("Got KILL signal. Shutdown.")
		// Close default stdin
		util.StopStdinScanner() // Close stdin and cause plugin context cancel
		return
	}

	// Check proto ver
	if base.Ver != cfg.Build.JsonVer && base.Ver != 0 {
		util.LogMsg("Warning: Event version (%d) does not match (%d)", base.Ver, cfg.Build.JsonVer)
	}

	switch base.Type {
	case "cmd":
		// If config is still empty, and JSON has "cfg" then initialize config
		if cfg.Self == nil && base.Cfg != nil {
			// Write data from JSON to fonfig
			cfg.Self = base.Cfg

			// Process config data
			cfg.LoadConfig(base.Path)

			// Update and start databases
			geomanager.StartGeoService(ctx)

			// Mark as ready to start
			cfg.Self.ReadyCfg = true
		}

	case "rkn": // Event with possible diffs to process
		// Hold function until bot client is ready
		if ret := cfg.WaitConfig(ctx); ret != true { // nil pointer guard
			return
		}
		hold := util.HoldAction(ctx, &cfg.Self.Ready, 24, 5)
		if !hold {
			util.LogMsg("ERROR: BOT CLIENT WAITTIME EXCEEDED!")
			util.StopStdinScanner() // Close stdin and cause plugin context cancel
			return
		}

		processRknEvent(ctx, base.Diff)

		if cfg.Build.Mode == "ONCE" {
			util.LogMsg("Event processed! Shutdown")
			// Close default stdin
			util.StopStdinScanner()
		}

	default:
		util.LogMsg("Unknown event type: %s", base.Type)
	}
}

// Put DiffObject results to sender blocks
func processRknEvent(ctx context.Context, diff *DiffObject) {
	// Initialize fields for converting channels from string to snowflake
	var (
		snowflake_ban snowflake.ID
		snowflake_unban snowflake.ID
		snowflake_banip snowflake.ID
		snowflake_unbanip snowflake.ID
		snowflake_total snowflake.ID
	)

	// If geoservices disabled then disable IP output
	if cfg.Self.NoMMDB {
		cfg.Sender.BanIp = false
		cfg.Sender.UnbanIp = false
	}

	// Parse snowflake to enabled senders. Sender type will be disabled if snowflake parser fails
	if cfg.Sender.Ban {
		snowflake_ban, cfg.Sender.Ban = util.ParseSnowflake(cfg.Channel.Ban)
	}
	if cfg.Sender.Unban {
		snowflake_unban, cfg.Sender.Unban = util.ParseSnowflake(cfg.Channel.Unban)
	}
	if cfg.Sender.BanIp {
		snowflake_banip, cfg.Sender.BanIp = util.ParseSnowflake(cfg.Channel.BanIp)
	}
	if cfg.Sender.UnbanIp {
		snowflake_unbanip, cfg.Sender.UnbanIp = util.ParseSnowflake(cfg.Channel.UnbanIp)
	}
	if cfg.Sender.Total {
		snowflake_total, cfg.Sender.Total = util.ParseSnowflake(cfg.Channel.Total)
	}

	// Read, Filter, split and send.
	// Perform order from fastest to longest task

	// If Daily Total enabled
	if cfg.Sender.Total {
		dissender.SendDailyStats(
			ctx, 
			*disclient.BotClient, 
			snowflake_total, 
			cfg.Data.TotalJSON,
			filepath.Join(cfg.Self.Path, "discord-sender", "marker"), 
			cfg.Embed.TotalClr,
		)
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// If BannedIP enabled and list not empty
	if cfg.Sender.BanIp && !diff.BannedIPs.Empty {
		dissender.ProcessIPs(
			ctx,
			*disclient.BotClient,
			snowflake_banip,
			cfg.Loc.RknAdded,
			diff.BannedIPs.Data,
			diff.BannedIPs.Length,
			diff.BannedIPs.Total,
			cfg.Embed.BanIpClr,
		)
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// If UnbannedIP enabled and list not empty
	if cfg.Sender.UnbanIp && !diff.UnbannedIPs.Empty {
		dissender.ProcessIPs(
			ctx,
			*disclient.BotClient,
			snowflake_unbanip,
			cfg.Loc.RknRemoved,
			diff.UnbannedIPs.Data,
			diff.UnbannedIPs.Length,
			0,
			cfg.Embed.UnbanIpClr,
		)
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// If Banned Domains enabled and list not empty
	if cfg.Sender.Ban && !diff.BannedDomains.Empty {
		dissender.ProcessDomains(
			ctx,
			*disclient.BotClient,
			snowflake_ban,
			cfg.Loc.RknAdded,
			diff.BannedDomains.Data,
			diff.BannedDomains.Length,
			diff.BannedDomains.Total,
			cfg.Embed.BanClr,
		)
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// If Unbanned Domains enabled and list not empty
	if cfg.Sender.Unban && !diff.UnbannedDomains.Empty {
		dissender.ProcessDomains(
			ctx,
			*disclient.BotClient,
			snowflake_unban,
			cfg.Loc.RknRemoved,
			diff.UnbannedDomains.Data,
			diff.UnbannedDomains.Length,
			0,
			cfg.Embed.UnbanClr,
		)
	}
}