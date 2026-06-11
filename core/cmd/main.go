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
	"zapretyan-go/internal/config"
	"zapretyan-go/internal/diffscanner"
	"zapretyan-go/internal/extensionhandler"
	"zapretyan-go/internal/extensionloader"
	"zapretyan-go/internal/flags"
	"zapretyan-go/internal/logger"
	"zapretyan-go/internal/sysservice"
	"zapretyan-go/internal/utils"
	"zapretyan-go/internal/sysinfo"
	// DEBUG
	// "zapretyan-go/internal/pprof"
)

const appVersion   string = "2.1.0.0" // App version
const jsonProtoVer int    = 1 		  // Version of JSON message payload

func main() {
	// DEBUG. CMD: "go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap"
	// Or browse http://localhost:8080/debug/pprof/goroutine?debug=1
	// go pprof.PprofStart()

	// Send start log with system info
	sysinfo.SendLogBanner(appVersion, jsonProtoVer)

	// Start logger and parse flags
	logger.SetupLogger()

	// Load Configuration
	config.InitConfig()
	// Set properties
	config.Params.Ver = appVersion
	config.Params.JsonVer = jsonProtoVer

	// Check for --install flag
	if flags.Args.Install {
		if err := sysservice.Install(); err != nil {
			slog.Error("Error installing service", "err", err)
		}
		utils.Pause()
		os.Exit(0)
	}
	// Check for --uninstall flag
	if flags.Args.Uninstall {
		if err := sysservice.Uninstall(); err != nil {
			slog.Error("Error uninstalling service", "err", err)
		}
		utils.Pause()
		os.Exit(0)
	}

	// Load configured extensions
	extensionloader.InitExtensions()

	// CORE CONTEXT:
	// Create Root Context for task cancelling
	ctx, cancel := context.WithCancel(context.Background())

	// Create Goroutines counter
	var wg sync.WaitGroup

	// If started with --run flag then we started as a system service
	if flags.Args.Service {
		wg.Add(1) // here wg.Done() is calling inside system specific files
		go sysservice.Run(ctx, cancel, &wg)
	}

	// Start STREAM extensions
	wg.Add(1)
	go extensionhandler.StartSteamExtensions(ctx, &wg)

	// Start difference scanner
	wg.Add(1)
	go diffscanner.Handler(ctx, &wg)

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
