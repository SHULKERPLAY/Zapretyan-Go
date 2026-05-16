package main

import (
	"fmt"
	"log/slog"
	"os"

	//Connection handler
	"bufio"
	"context"
	"net"
	"os/signal"
	"strings"
	"syscall"
	"time"

	//Internal
	"zapretyan-go/internal/logger"
	"zapretyan-go/internal/config"
)

func main() {
	// Default params
	config.Params.Ver = "0.1.0"

	// Start logger and parse flags
	logger.SetupLogger()

	slog.Info("ZAPRETYAN-GO CORE", "ver", config.Params.Ver)
	defer slog.Info("App closed")
}
