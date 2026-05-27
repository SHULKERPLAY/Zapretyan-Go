package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"zapretyan-go/internal/flags"

	"github.com/lmittmann/tint"
	"gopkg.in/natefinch/lumberjack.v2"
)

func SetupLogger() {
	defer slog.Debug("setupLogger() ended")

	// Parse App Flags
	flags.ParseFlags()

	// Default Level: Info
	level := slog.LevelInfo
	addsource := false
	// Selecting Level
	if flags.Args.Loglevel != "" {
		switch flags.Args.Loglevel {
		case "debug":
			level = slog.LevelDebug
			addsource = true
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	// Check if logfile enabled
	writer, noclr := logRotate(os.Stderr)

	//init logger
	logger := slog.New(tint.NewHandler(writer, &tint.Options{
		Level:      level,
		TimeFormat: "15:04:05",
		NoColor:    noclr, 		// Color disabled if log to file enabled 
		AddSource:  addsource,
	}))
	slog.SetDefault(logger)
	slog.Info("Log level", "level", level)
}

// Checking logfile flag and return io.Writer property to output logs in.
// Returns second bool for disabling color if log in file enabled. False if console
// dOutput argument accepts io.Writes such as os.Stdout to return it by default.
// If flag not set returns the stream that was specified as default.
func logRotate(dOutput io.Writer) (io.Writer, bool) {
	// If flag not set
	if !flags.Args.LogFile {
		// Fallback to Console-Only output
		return dOutput, false
	}
	logPath, _ := filepath.Abs("./logs/zapretyan.log")
	slog.Info("", "logfile", logPath)

	// Check if we can write to this directory
	dir := filepath.Dir(logPath)
	// Create directory if not existing
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("[LOGGER] FATAL: NO PERMISSIONS TO CREATE DIR", "dir", dir, "err", err)
		os.Exit(1)
	}

	// Check logfile access
	testFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		// If file protected from write (Permission denied)
		slog.Error("[LOGGER] Cannot write to file. Fallback to Console-Only logs.", "file", logPath, "err", err)

		// Fallback to Console-Only output
		return dOutput, false
	}
	testFile.Close() // Test success

	// Lumberjack automaticly creates a file and tracking it size
	logRoller := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    8,    // Max filesize in MB to new rotation
		MaxBackups: 5,    // Store max amount of files
		MaxAge:     90,   // Store logs this amount of days
		Compress:   true, // Compress old logs into tar.gz (GZip)
	}

	// Create combined stream to broadcast logs into console and logfile
	w := io.MultiWriter(logRoller, dOutput)
	return w, true
}