// Package storage persists the domain in a single SQLite file using the pure
// Go driver modernc.org/sqlite, so builds need no C toolchain. Every write
// goes through a transaction; the event log is appended inside the same
// transaction as the change it describes, which is what lets the UI replay
// events after a reconnect without missing or duplicating any.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("storage: not found")

// ErrSchemaTooNew is returned when the database was migrated by a newer
// build than this one. Opening it would risk writing rows the newer schema
// does not expect, so the build refuses instead of guessing.
var ErrSchemaTooNew = errors.New("storage: database schema is newer than this build")

// DB is an open database.
type DB struct {
	sql  *sql.DB
	path string
}

// Open opens or creates the database at path and applies pending
// migrations. Use ":memory:" only for throwaway tests: WAL mode and
// multiple connections need a file.
func Open(ctx context.Context, path string) (*DB, error) {
	handle, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	if path == ":memory:" {
		handle.SetMaxOpenConns(1)
	}
	db := &DB{sql: handle, path: path}
	if err := db.Migrate(ctx); err != nil {
		_ = handle.Close()
		return nil, err
	}
	return db, nil
}

// dsn builds the connection string shared by Open and OpenExisting.
//
// _txlock=immediate makes read-write transactions take the write lock at
// BEGIN, so a second writer waits for busy_timeout instead of failing with
// SQLITE_BUSY when it tries to upgrade a read snapshot. Read-only
// transactions (View) still begin deferred.
func dsn(path string) string {
	return "file:" + path + "?" + url.Values{
		"_txlock": []string{"immediate"},
		"_pragma": []string{
			"journal_mode(WAL)",
			"foreign_keys(ON)",
			"busy_timeout(5000)",
			"synchronous(NORMAL)",
		},
	}.Encode()
}

// OpenExisting opens a database without migrating it, for tools that run
// next to the application. The file must exist and carry exactly the
// schema version this build knows: an older file is migrated by the
// application on its next start, a newer one belongs to a newer build.
func OpenExisting(ctx context.Context, path string) (*DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("storage: open existing: %w", err)
	}
	handle, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	db := &DB{sql: handle, path: path}
	have, err := db.SchemaVersion(ctx)
	if err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("storage: %s is not an Agent Forum database: %w", path, err)
	}
	want, err := latestVersion(migrationFS, "migrations")
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	switch {
	case have > want:
		_ = handle.Close()
		return nil, fmt.Errorf("%w: database has version %d, this build knows versions up to %d", ErrSchemaTooNew, have, want)
	case have < want:
		_ = handle.Close()
		return nil, fmt.Errorf("storage: database has schema version %d, this build expects %d; start the application once to migrate it", have, want)
	}
	return db, nil
}

func latestVersion(fsys fs.FS, dir string) (int, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, e := range entries {
		v, err := migrationVersion(e.Name())
		if err != nil {
			return 0, err
		}
		latest = max(latest, v)
	}
	return latest, nil
}

// Close closes the database.
func (db *DB) Close() error { return db.sql.Close() }

// Path returns the file path the database was opened with.
func (db *DB) Path() string { return db.path }

// Migrate applies the embedded migrations that have not been applied yet.
// Each migration runs in its own transaction and is recorded in
// schema_migrations, so a crash between two migrations is safe to retry.
//
// Before the first pending migration runs on a database that already holds
// a schema, the file is backed up next to itself as
// "<name>.pre-v<N>-<timestamp>.bak" and the copy is verified. A database
// migrated by a newer build is refused with ErrSchemaTooNew.
func (db *DB) Migrate(ctx context.Context) error {
	return db.migrate(ctx, migrationFS, "migrations")
}

type migration struct {
	version int
	name    string
}

func (db *DB) migrate(ctx context.Context, fsys fs.FS, dir string) error {
	if _, err := db.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("storage: migrations table: %w", err)
	}
	applied := map[int]bool{}
	rows, err := db.sql.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	known := 0
	var pending []migration
	for _, e := range entries {
		version, err := migrationVersion(e.Name())
		if err != nil {
			return err
		}
		known = max(known, version)
		if !applied[version] {
			pending = append(pending, migration{version: version, name: e.Name()})
		}
	}
	for v := range applied {
		if v > known {
			return fmt.Errorf("%w: database has version %d, this build knows versions up to %d", ErrSchemaTooNew, v, known)
		}
	}
	if len(pending) > 0 && len(applied) > 0 && db.path != ":memory:" {
		dest := fmt.Sprintf("%s.pre-v%d-%s.bak", db.path, pending[0].version, time.Now().UTC().Format("20060102T150405Z"))
		if err := db.Backup(ctx, dest); err != nil {
			return fmt.Errorf("storage: backup before migration %s: %w", pending[0].name, err)
		}
	}

	for _, m := range pending {
		body, err := fs.ReadFile(fsys, dir+"/"+m.name)
		if err != nil {
			return err
		}
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("storage: migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, formatTime(time.Now())); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration version.
func (db *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := db.sql.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	return int(v.Int64), err
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("storage: migration %q has no numeric prefix", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("storage: migration %q: %w", name, err)
	}
	return v, nil
}

// Tx is a transaction scoped to a context. All repository methods hang off
// it so that a change and its events are committed together.
type Tx struct {
	ctx context.Context
	tx  *sql.Tx
}

// Update runs fn in a read-write transaction and commits when fn returns
// nil. Any error rolls the transaction back.
func (db *DB) Update(ctx context.Context, fn func(*Tx) error) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin: %w", err)
	}
	if err := fn(&Tx{ctx: ctx, tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit: %w", err)
	}
	return nil
}

// View runs fn in a transaction that is always rolled back, for consistent
// reads.
func (db *DB) View(ctx context.Context, fn func(*Tx) error) error {
	tx, err := db.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("storage: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return fn(&Tx{ctx: ctx, tx: tx})
}

// timeLayout is fixed-width so that lexical order of the stored strings
// equals chronological order; RFC3339Nano trims trailing zeros and breaks
// that property.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
