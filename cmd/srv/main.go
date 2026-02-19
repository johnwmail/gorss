package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/johnwmail/gorss/db"
	"github.com/johnwmail/gorss/srv"
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
	flagRestore = flag.String("restore", "", "restore database from backup file and exit")
	flagBackup  = flag.String("backup", "", "create a one-time backup to the given directory and exit")
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

	// Determine DB path
	dbPath := "db.sqlite3"
	if envPath := os.Getenv("GORSS_DB_PATH"); envPath != "" {
		dbPath = envPath
	}

	// Restore command
	if *flagRestore != "" {
		fmt.Printf("Restoring database from: %s\n", *flagRestore)
		fmt.Printf("Target database: %s\n", dbPath)
		fmt.Println("")
		fmt.Println("WARNING: This will replace the existing database.")
		fmt.Println("Make sure the GoRSS server is stopped before restoring.")
		fmt.Print("Continue? [y/N] ")
		var answer string
		_, _ = fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Restore cancelled.")
			return nil
		}
		if err := db.Restore(*flagRestore, dbPath); err != nil {
			return fmt.Errorf("restore failed: %w", err)
		}
		fmt.Println("Restore complete. You can start the server now.")
		return nil
	}

	// One-time backup command
	if *flagBackup != "" {
		srcDB, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer srcDB.Close() //nolint:errcheck
		path, err := db.Backup(srcDB, *flagBackup)
		if err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
		fmt.Printf("Backup saved to: %s\n", path)
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
	server, err := srv.New(dbPath, hostname, Version)
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
  GORSS_BACKUP_DIR          Directory for periodic backups (disabled if unset)
  GORSS_BACKUP_INTERVAL     Backup interval, e.g. 12h, 24h (default: 24h)
  GORSS_BACKUP_KEEP         Number of backup files to keep (default: 7)
  TZ                        Timezone (default: UTC)

Examples:
  gorss                                    # Run with defaults
  gorss -port 3000                         # Listen on port 3000
  gorss -backup /backups                   # Create a one-time backup
  gorss -restore /backups/gorss-2026-02-16-030000.db  # Restore from backup
  GORSS_BACKUP_DIR=/backups gorss          # Enable periodic backup every 24h
  GORSS_AUTH_MODE=password GORSS_PASSWORD=secret gorss
  GORSS_DB_PATH=/data/gorss.db GORSS_REFRESH_INTERVAL=30m gorss
`)
}
