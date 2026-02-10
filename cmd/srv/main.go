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

var (
	flagPort    = flag.String("port", "8080", "port to listen on")
	flagVersion = flag.Bool("version", false, "print version and exit")
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = usage
	flag.Parse()

	if *flagVersion {
		fmt.Printf("gorss %s (commit: %s, built: %s)\n", Version, CommitHash, BuildTime)
		return nil
	}

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

func usage() {
	fmt.Fprintf(os.Stderr, `GoRSS - A self-hosted RSS/Atom feed reader

Version:    %s
Commit:     %s
Build Time: %s

Usage:
  gorss [flags]

Flags:
`, Version, CommitHash, BuildTime)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Environment Variables:
  GORSS_PORT                Port to listen on (default: 8080)
  GORSS_DB_PATH             Path to SQLite database (default: ./db.sqlite3)
  GORSS_AUTH_MODE           Authentication mode: none, password, proxy (default: none)
  GORSS_PASSWORD            Password for "password" auth mode
  GORSS_REFRESH_INTERVAL    Feed refresh interval, e.g. 30m, 1h, 2h (default: 1h)
  GORSS_PURGE_DAYS          Auto-purge read articles older than N days, 0 to disable (default: 30)
  TZ                        Timezone (default: UTC)

Examples:
  gorss                                    # Run with defaults
  gorss -port 3000                         # Listen on port 3000
  GORSS_AUTH_MODE=password GORSS_PASSWORD=secret gorss
  GORSS_DB_PATH=/data/gorss.db GORSS_REFRESH_INTERVAL=30m gorss
`)
}
