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

// DB is an open database.
type DB struct {
	sql  *sql.DB
	path string
}

// Open opens or creates the database at path and applies pending
// migrations. Use ":memory:" only for throwaway tests: WAL mode and
// multiple connections need a file.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := "file:" + path + "?" + url.Values{
		"_pragma": []string{
			"journal_mode(WAL)",
			"foreign_keys(ON)",
			"busy_timeout(5000)",
			"synchronous(NORMAL)",
		},
	}.Encode()
	handle, err := sql.Open("sqlite", dsn)
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

// Close closes the database.
func (db *DB) Close() error { return db.sql.Close() }

// Path returns the file path the database was opened with.
func (db *DB) Path() string { return db.path }

// Migrate applies the embedded migrations that have not been applied yet.
// Each migration runs in its own transaction and is recorded in
// schema_migrations, so a crash between two migrations is safe to retry.
func (db *DB) Migrate(ctx context.Context) error {
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

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		version, err := migrationVersion(e.Name())
		if err != nil {
			return err
		}
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("storage: migration %s: %w", e.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			version, e.Name(), formatTime(time.Now())); err != nil {
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

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
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
