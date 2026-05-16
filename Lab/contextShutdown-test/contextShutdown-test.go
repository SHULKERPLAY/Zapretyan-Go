package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	// Create Root Context for task cancelling
	ctx, cancel := context.WithCancel(context.Background())

	// Create Goroutines counter
	var wg sync.WaitGroup

	// Start any background goroutines
	wg.Add(1)
	go periodicScanner(ctx, &wg) // Test 1: Periodic timer check

	wg.Add(1)
	go heavyDataProcessor(ctx, &wg) // Test 2: Heavy sequence processing

	wg.Add(1)
	go goroutinesDeep(ctx, &wg) // Test: Deep Goroutines

	// Call blocking function for catching interrupts
	// Giving to function the cancel remote (Cancel) and task counter (wg)
	WaitForSystemSignals(cancel, &wg)
}

// WaitForSystemSignals BLOCKS THE PROGRAM and waiting for interrupts.
// Coordinating Graceful Shutdown of all components
func WaitForSystemSignals(cancel context.CancelFunc, wg *sync.WaitGroup) {
	sigCh := make(chan os.Signal, 1)
	// Catch interrupts
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait until system signal arrives
	sig := <-sigCh
	slog.Warn("GOT INTERRUPT!", "signal", sig.String())

	slog.Info("Starting Graceful Shutdown. Notify goroutines...")
	cancel() // Send shutdown with ctx.Done() in all functions

	// Create channel for strict shutdown timeout
	shutdownDone := make(chan struct{})
	go func() {
		wg.Wait() // Wait until all goroutines call wg.Done()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		slog.Info("All goroutines successfuly stopped! Graceful Shutdown.")
	case <-time.After(10 * time.Second):
		slog.Error("TIMEOUT! Some functions did not stopped in time. Force shutdown.")
	}
}

// Test 1
func periodicScanner(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done() // decrement WaitGroup counter when function ends

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop() // Clean timer resources

	for {
		select {
		case <-ctx.Done():
			// Graceful Shutdown
			slog.Info("[Scanner] Recieved interrupt. Cleaning buffers and shutdown...")
			return // Function end

		case <-ticker.C:
			// If context open do simple job every 3 seconds
			slog.Info("[Scanner] Check for new changes...")
		}
	}
}

// Test 2
func heavyDataProcessor(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	mockData := []string{"chunk1", "chunk2", "chunk3", "chunk4", "chunk5"}

	for _, chunk := range mockData {
		// CRITICAL: Check if context is closed
		// before processing next chunk of data
		if ctx.Err() != nil {
			slog.Warn("[Processor] Stop data processing: Core shutdown.")
			return
		}

		// Heavy load
		slog.Info("[Processor] Processing data block...", "chunk", chunk)
		time.Sleep(1500 * time.Millisecond)
	}

	slog.Info("[Processor] All processes completed successfuly.")
}

// Deep Goroutines
func goroutinesDeep(ctx context.Context, wg *sync.WaitGroup) {
	// defer wg.Done()
	// If this is a transit function we not calling wg.Done()
	// We need to pass sync.WaitGroup pointer to deepest function that need to shutdown

	slog.Info("Some actions")
	time.Sleep(1500 * time.Millisecond)
	slog.Info("Actions completed")

	// It is already pointer so we dont neet to type (&wg)
	// Context pass to all deeper functions and goroutines
	goDeeper(ctx, wg)
}

func goDeeper(ctx context.Context, GlobalWg *sync.WaitGroup) {
	defer GlobalWg.Done() // When all local workgroups has stopped we send wg.Done() to top function

	// We need to wait all goroutines in this block. So we create seperate WaitGroup
	// And wait until everything in local workgroup will stop.
	// Then function ends and sending GlobalWg.Done()
	var localWg sync.WaitGroup

	mockData := []string{"chunk1", "chunk2", "chunk3", "chunk4", "chunk5"}
	// Starting STREAM extensions
	for range mockData {
		// Local workgoup starts 5 processes
		localWg.Add(1)
		// Pass context inside. Function must have localWg.Done() inside and stop work when <-ctx.Done() or ctx.Err() != nil
		go heavyDataProcessor(ctx, &localWg)
	}

	// Wait until all plugins stop
	localWg.Wait()
	slog.Info("All localWg stopped.")
}
