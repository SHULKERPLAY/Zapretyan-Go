package logger

import (
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
	"zapretyan-go/internal/flags"
)

func SetupLogger() {
	defer slog.Debug("setupLogger() ended")
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
	
	//init logger
	logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      level,
		TimeFormat: "15:04:05",
		NoColor:    false,
		AddSource:  addsource,
	}))
	slog.SetDefault(logger)
	slog.Info("Log level", "level", level)
}
