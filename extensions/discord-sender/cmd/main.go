package main

import (
	"context"
	"discord-sender/internal/cfg"
	"discord-sender/internal/disclient"
	"discord-sender/internal/events"
	"discord-sender/internal/util"
	"sync"
)

const pver string = "1.0.0@36hbsug1" // App version
const pjver int = 1                  // Version of JSON message payload
const pmode string = "ONCE"          // Plugin mode

func main() {
	// Init basic params
	cfg.Build.JsonVer = pjver
	cfg.Build.Ver = pver
	cfg.Build.Mode = pmode

	// Create waitgroup for tasks
	var wg sync.WaitGroup

	// Create Root Context for task cancelling
	ctx, cancel := context.WithCancel(context.Background())

	// Start core events listener (Config will be loaded from config event)
	wg.Add(1)
	go events.CoreListener(ctx, cancel, &wg)

	// Start Discord Client
	wg.Add(1)
	go disclient.Start(ctx, &wg)

	// Attribution
	util.LogMsg("Default IP Geolocation by DB-IP: db-ip.com")

	wg.Wait() // Wait for goroutines to end
	util.LogMsg("Correct plugin shutdown")
}