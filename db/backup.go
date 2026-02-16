package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup creates a copy of the database using SQLite's built-in backup mechanism.
// The backup file is named gorss-YYYY-MM-DD-HHMMSS.db in the given directory.
func Backup(srcDB *sql.DB, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	filename := fmt.Sprintf("gorss-%s.db", timestamp)
	dstPath := filepath.Join(backupDir, filename)

	// Use SQLite's VACUUM INTO for a safe online backup.
	// This creates a clean, standalone copy even while the app is running.
	if _, err := srcDB.Exec("VACUUM INTO ?", dstPath); err != nil {
		// Clean up partial file on failure
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("vacuum into %s: %w", dstPath, err)
	}

	return dstPath, nil
}

// PruneBackups keeps only the most recent 'keep' backup files in the directory.
func PruneBackups(backupDir string, keep int) error {
	if keep <= 0 {
		return nil
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("read backup dir: %w", err)
	}

	// Collect backup files (gorss-*.db)
	var backups []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "gorss-") && strings.HasSuffix(name, ".db") {
			backups = append(backups, name)
		}
	}

	// Sort alphabetically (timestamp in name ensures chronological order)
	sort.Strings(backups)

	// Remove oldest files beyond the keep count
	if len(backups) <= keep {
		return nil
	}
	for _, name := range backups[:len(backups)-keep] {
		path := filepath.Join(backupDir, name)
		if err := os.Remove(path); err != nil {
			slog.Warn("failed to remove old backup", "path", path, "error", err)
		} else {
			slog.Info("pruned old backup", "file", name)
		}
	}
	return nil
}

// Restore copies a backup file to the target database path.
// The target DB must NOT be open — stop the server first.
// This also removes any stale -wal and -shm files.
func Restore(backupPath, targetDBPath string) error {
	// Verify backup file exists and is readable
	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("backup file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("backup path is a directory, not a file")
	}
	if info.Size() == 0 {
		return fmt.Errorf("backup file is empty")
	}

	// Validate it's a real SQLite database by opening it
	testDB, err := sql.Open("sqlite", backupPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open backup for validation: %w", err)
	}
	if err := testDB.Ping(); err != nil {
		_ = testDB.Close()
		return fmt.Errorf("backup file is not a valid SQLite database: %w", err)
	}
	_ = testDB.Close()

	// Remove existing DB + WAL/SHM files
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := targetDBPath + suffix
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}

	// Copy backup to target
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	if err := os.WriteFile(targetDBPath, data, 0o644); err != nil {
		return fmt.Errorf("write target: %w", err)
	}

	return nil
}
