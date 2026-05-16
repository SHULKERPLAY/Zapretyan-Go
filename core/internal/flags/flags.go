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

func init() {
	// Init Args
	Args = &AppArgs{}
	parseFlags()
}

func parseFlags() {
	slog.Debug("Parsing flags...")

	// Define flags (name, default, description)
	defer slog.Debug("parseFlags() ended")
	flag.StringVar(&Args.Loglevel, "log", "", "Set log level: 'info', 'warn', 'error' or 'debug'")
	flag.Parse()

	slog.Debug("Got flags.", "flags", Args)
}

// Usage: flagRequired(Args.Loglevel)
func flagRequired(test string) {
	defer slog.Debug("flagRequired() ended")
	slog.Debug("Reqired!", "flag", test)
	if test == "" {
		slog.Error("Missing required arguments!")
		flag.Usage()
		os.Exit(1)
	}
}
