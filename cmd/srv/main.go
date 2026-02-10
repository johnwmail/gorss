package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"srv.exe.dev/srv"
)

// Build-time variables, injected via -ldflags.
var (
	Version    = "vDev"
	BuildTime  = "timeless"
	CommitHash = "sha-unknown"
)

var flagPort = flag.String("port", "8080", "port to listen on")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func run() error {
	flag.Parse()

	slog.Info("gorss starting",
		"Version", Version,
		"CommitHash", CommitHash,
		"BuildTime", BuildTime,
	)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	server, err := srv.New("db.sqlite3", hostname)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	return server.Serve(*flagPort)
}
