package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: create a temp DB with a simple table and some rows.
func newTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, val TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < 5; i++ {
		_, err = db.Exec(`INSERT INTO items (val) VALUES (?)`, "item")
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dbPath
}

func TestBackup(t *testing.T) {
	db, _ := newTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	path, err := Backup(db, backupDir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// File must exist and be non-empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}

	// Must be a valid SQLite DB with our data.
	bkDB, err := Open(path)
	if err != nil {
		t.Fatalf("open backup db: %v", err)
	}
	defer bkDB.Close()

	var count int
	if err := bkDB.QueryRow(`SELECT count(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 rows, got %d", count)
	}
}

func TestBackupCreatesDir(t *testing.T) {
	db, _ := newTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "a", "b", "c")

	_, err := Backup(db, backupDir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("stat backup dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("backup dir is not a directory")
	}
}

func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()

	// Create 5 fake backup files with lexicographic ordering.
	names := []string{
		"gorss-2024-01-01-100000.db",
		"gorss-2024-02-01-100000.db",
		"gorss-2024-03-01-100000.db",
		"gorss-2024-04-01-100000.db",
		"gorss-2024-05-01-100000.db",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Also create a non-backup file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := PruneBackups(dir, 2); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	// The two newest should remain.
	for _, n := range names[:3] {
		if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be pruned", n)
		}
	}
	for _, n := range names[3:] {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("expected %s to still exist: %v", n, err)
		}
	}
	// Non-backup file untouched.
	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Error("non-backup file should not be removed")
	}
}

func TestPruneBackupsKeepZero(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"gorss-2024-01-01-100000.db",
		"gorss-2024-02-01-100000.db",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := PruneBackups(dir, 0); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	// keep=0 means "no pruning", so all files should remain.
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("expected %s to still exist", n)
		}
	}
}

func TestPruneBackupsNoDir(t *testing.T) {
	err := PruneBackups(filepath.Join(t.TempDir(), "nope"), 3)
	if err == nil {
		t.Fatal("expected error for non-existent dir")
	}
}

func TestRestore(t *testing.T) {
	db, _ := newTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	backupPath, err := Backup(db, backupDir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Restore to a fresh path.
	targetPath := filepath.Join(t.TempDir(), "restored.db")
	if err := Restore(backupPath, targetPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredDB, err := Open(targetPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restoredDB.Close()

	var count int
	if err := restoredDB.QueryRow(`SELECT count(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("query restored db: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 rows, got %d", count)
	}
}

func TestRestoreInvalidFile(t *testing.T) {
	// Write a non-SQLite file.
	bad := filepath.Join(t.TempDir(), "bad.db")
	if err := os.WriteFile(bad, []byte("not a sqlite db"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "target.db")
	err := Restore(bad, target)
	if err == nil {
		t.Fatal("expected error for non-SQLite file")
	}
}

func TestRestoreNonExistent(t *testing.T) {
	err := Restore(filepath.Join(t.TempDir(), "nope.db"), filepath.Join(t.TempDir(), "target.db"))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestRestoreDirectory(t *testing.T) {
	dir := t.TempDir()
	err := Restore(dir, filepath.Join(t.TempDir(), "target.db"))
	if err == nil {
		t.Fatal("expected error when backup path is a directory")
	}
}

func TestRestoreCleansWalShm(t *testing.T) {
	// Create a real backup to restore from.
	db, _ := newTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	backupPath, err := Backup(db, backupDir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "app.db")

	// Create fake -wal and -shm files at the target.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(targetPath+suffix, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Restore(backupPath, targetPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// The -wal and -shm files must be gone.
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(targetPath + suffix); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", targetPath+suffix)
		}
	}

	// The main DB file should be the restored content, not "stale".
	restoredDB, err := Open(targetPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restoredDB.Close()

	var count int
	if err := restoredDB.QueryRow(`SELECT count(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("query restored db: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 rows, got %d", count)
	}
}

func TestLatestBackupAge_NoDir(t *testing.T) {
	age := LatestBackupAge(filepath.Join(t.TempDir(), "nope"))
	if age < 24*time.Hour {
		t.Fatalf("expected very large age for missing dir, got %v", age)
	}
}

func TestLatestBackupAge_Empty(t *testing.T) {
	dir := t.TempDir()
	age := LatestBackupAge(dir)
	if age < 24*time.Hour {
		t.Fatalf("expected very large age for empty dir, got %v", age)
	}
}

func TestLatestBackupAge_Recent(t *testing.T) {
	dir := t.TempDir()
	// Create a backup file with a timestamp of ~2 minutes ago.
	ts := time.Now().Add(-2 * time.Minute).Format("2006-01-02-150405")
	name := fmt.Sprintf("gorss-%s.db", ts)
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	age := LatestBackupAge(dir)
	if age > 5*time.Minute {
		t.Fatalf("expected age ~2m, got %v", age)
	}
	if age < 1*time.Minute {
		t.Fatalf("expected age ~2m, got %v", age)
	}
}

func TestLatestBackupAge_PicksNewest(t *testing.T) {
	dir := t.TempDir()
	// Old backup
	if err := os.WriteFile(filepath.Join(dir, "gorss-2020-01-01-120000.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Recent backup (~1 minute ago)
	ts := time.Now().Add(-1 * time.Minute).Format("2006-01-02-150405")
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("gorss-%s.db", ts)), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	age := LatestBackupAge(dir)
	if age > 5*time.Minute {
		t.Fatalf("expected age ~1m, got %v", age)
	}
}
