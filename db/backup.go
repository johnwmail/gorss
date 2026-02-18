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

const maxDuration = time.Duration(1<<63 - 1)

// LatestBackupAge returns the age of the most recent valid backup in the
// directory, or a very large duration if no valid backups exist.
// A backup is considered valid only if its filename timestamp parses correctly,
// the file is non-empty, and it can be opened and pinged as a SQLite database.
func LatestBackupAge(backupDir string) time.Duration {
	age, _ := latestValidBackup(backupDir)
	return age
}

// latestValidBackup finds the most recent backup file (by filename timestamp),
// validates it is a real SQLite database, and returns its age and path.
// If no valid backup is found, age is maxDuration and path is empty.
func latestValidBackup(backupDir string) (time.Duration, string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return maxDuration, ""
	}

	// Collect and sort backup filenames newest-first so we can validate
	// the most recent ones first and return early.
	type candidate struct {
		name string
		ts   time.Time
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "gorss-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		ts := strings.TrimPrefix(name, "gorss-")
		ts = strings.TrimSuffix(ts, ".db")
		t, err := time.Parse("2006-01-02-150405", ts)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{name, t})
	}

	// Sort newest first
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ts.After(candidates[j].ts)
	})

	for _, c := range candidates {
		path := filepath.Join(backupDir, c.name)

		// Check file is non-empty
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			slog.Warn("backup file empty or unreadable, skipping", "file", c.name)
			continue
		}

		// Validate it's a real SQLite database
		testDB, err := sql.Open("sqlite", path+"?mode=ro")
		if err != nil {
			slog.Warn("backup file cannot be opened as SQLite, skipping", "file", c.name, "error", err)
			continue
		}
		err = testDB.Ping()
		_ = testDB.Close()
		if err != nil {
			slog.Warn("backup file failed SQLite ping, skipping", "file", c.name, "error", err)
			continue
		}

		return time.Since(c.ts), path
	}

	return maxDuration, ""
}

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
