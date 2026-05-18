package flags

import (
	"flag"
	"log/slog"
	"os"
)

var Args *AppArgs

type AppArgs struct {
	Loglevel    string
}

func ParseFlags() {
	defer slog.Debug("ParseFlags() ended")
	// Init Args
	Args = &AppArgs{}

	slog.Info("Parsing flags...")

	// Define flags (name, default, description)
	flag.StringVar(&Args.Loglevel, "log", "", "Set log level: 'info', 'warn', 'error' or 'debug'")
	flag.Parse()

	slog.Debug("Got flags.", "flags", Args)
}

// Usage: flagRequired(Args.Loglevel)
func flagRequired(test string) {
	defer slog.Debug("flagRequired() ended")
	slog.Debug("Required!", "flag", test)
	if test == "" {
		slog.Error("FATAL: Missing required arguments!")
		flag.Usage()
		os.Exit(1)
	}
}
