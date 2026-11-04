package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Backup writes a consistent copy of the open database to dest and
// verifies it. The copy is made with VACUUM INTO, which reads one snapshot
// while the application keeps running, and is checked with
// PRAGMA integrity_check and the schema version. dest must not exist; a
// copy that fails verification is removed, so a backup file on disk is
// always a good one.
//
// To restore, close the application, move the current database aside
// together with its -wal and -shm files, and copy the backup in its place.
func (db *DB) Backup(ctx context.Context, dest string) error {
	if db.path == ":memory:" {
		return errors.New("storage: cannot back up an in-memory database")
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("storage: backup target %s already exists", dest)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("storage: backup target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return fmt.Errorf("storage: backup directory: %w", err)
	}
	version, err := db.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("storage: backup: schema version: %w", err)
	}
	if _, err := db.sql.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("storage: backup: %w", err)
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("storage: backup permissions: %w", err)
	}
	if err := VerifyBackup(ctx, dest, version); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

// VerifyBackup opens a backup file read-only and checks that it passes
// SQLite's integrity check and carries the expected schema version. A
// wantVersion of zero skips the version comparison.
func VerifyBackup(ctx context.Context, path string, wantVersion int) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("storage: verify backup: %w", err)
	}
	// immutable=1: the file is a closed snapshot that nothing else writes,
	// so SQLite can read it without creating -wal or -shm files.
	handle, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return fmt.Errorf("storage: verify backup: %w", err)
	}
	defer func() { _ = handle.Close() }()
	var result string
	if err := handle.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("storage: verify backup: integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("storage: verify backup: integrity check reported %q", result)
	}
	var version sql.NullInt64
	if err := handle.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("storage: verify backup: schema version: %w", err)
	}
	if wantVersion > 0 && int(version.Int64) != wantVersion {
		return fmt.Errorf("storage: verify backup: schema version %d, want %d", version.Int64, wantVersion)
	}
	return nil
}
