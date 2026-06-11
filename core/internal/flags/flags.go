package flags

import (
	"flag"
	"log/slog"
	"os"
)

var Args *AppArgs

type AppArgs struct {
	Loglevel    string
	LogFile 	bool    // Whether to write logs into stderr + file
	LogNoclr 	bool 	// Disable color in output logs
	DumpEvent 	bool
	Install 	bool
	Uninstall 	bool
	Service 	bool
}

func ParseFlags() {
	defer slog.Debug("ParseFlags() ended")
	// Init Args
	Args = &AppArgs{}

	slog.Info("Parsing flags...")

	// Define flags (name, default, description)
	flag.StringVar(&Args.Loglevel, "log", "", "Set log level: 'info', 'warn', 'error' or 'debug'")
	flag.BoolVar(&Args.LogFile, "logfile", false, "Save app logs into ./logs folder. Automaticly disables colors in log")
	flag.BoolVar(&Args.LogNoclr, "nocolor", false, "Disable colog for log output.")
	flag.BoolVar(&Args.DumpEvent, "dumpevent", false, "DEBUG: Write every first plugin event json into ./data/debug folder")
	flag.BoolVar(&Args.Install, "install", false, "Install Zapretyan-Go core as a system service (Autostart)")
	flag.BoolVar(&Args.Uninstall, "uninstall", false, "Uninstall existing Zapretyan-Go system service")
	flag.BoolVar(&Args.Service, "run", false, "Technical flag. Core signal to work in system service mode")
	flag.Parse()

	// Flags override
	if Args.Install {
		Args.LogNoclr = true
	}
	if Args.Uninstall {
		Args.LogNoclr = true
	}
	if Args.Service {
		Args.LogFile = true
		Args.LogNoclr = true
	}
	if Args.LogFile {
		Args.LogNoclr = true
	}
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
