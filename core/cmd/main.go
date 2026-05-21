package main

import (
	//Internal
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"zapretyan-go/internal/logger"
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/extensionhandler"
	"zapretyan-go/internal/extensionloader"
)

func main() {
	// Start logger and parse flags
	logger.SetupLogger()

	// Load Configuration
	config.InitConfig()

	// Default params
	config.Params.Ver = "0.1.0"
	// Version of JSON message payload
	config.Params.JsonVer = 1
	
	// Load configured extensions
	extensionloader.InitExtensions()

	// CORE CONTEXT:
	// Create Root Context for task cancelling
	ctx, cancel := context.WithCancel(context.Background())

	// Create Goroutines counter
	var wg sync.WaitGroup

	// Start STREAM extensions
	wg.Add(1)
	go extensionhandler.StartSteamExtensions(ctx, &wg)

	// Start difference scanner
	// wg.Add(1)
	// go diffscanner.Handler(ctx, &wg)

	// Call blocking function for catching interrupts
	// Giving to function the cancel remote (Cancel) and task counter (wg)
	WaitForSystemSignals(cancel, &wg)
}

func WaitForSystemSignals(cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer slog.Debug("WaitForSystemSignals() ended")
	sigCh := make(chan os.Signal, 1)

	// Catch interrupts
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait until system signal arrives
	sig := <-sigCh
	slog.Warn("GOT INTERRUPT!", "signal", sig.String())

	slog.Info("Starting core Shutdown. Notify modules...")
	cancel() // Send shutdown with ctx.Done() in all functions

	// Create channel for strict shutdown timeout
	shutdownDone := make(chan struct{})
	go func() {
		wg.Wait() // Wait until all goroutines call wg.Done()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		slog.Info("All modules successfuly stopped!")
	case <-time.After(10 * time.Second):
		slog.Warn("TIMEOUT! Some functions did not stopped in time. Force shutdown.")
	}
}
