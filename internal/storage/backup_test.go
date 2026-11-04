package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
)

func TestBackupIsConsistentVerifiedAndRestorable(t *testing.T) {
	db := openTest(t)
	_, tp, _ := seed(t, db)
	ctx := context.Background()
	err := db.Update(ctx, func(tx *Tx) error {
		return tx.CreateMessage(domain.Message{ID: "m1", TopicID: tp.ID, AuthorKind: domain.AuthorHuman, Body: "keep me", Visibility: domain.VisibilityVisible, CreatedAt: time.Now()})
	})
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "backups", "forum-copy.db")
	if err := db.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("backup permissions %o, want 600", perm)
	}
	if err := VerifyBackup(ctx, dest, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Backup(ctx, dest); err == nil {
		t.Fatal("an existing backup must not be overwritten")
	}

	// Restoring is opening the copy as the database.
	restored, err := Open(ctx, dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	_ = restored.View(ctx, func(tx *Tx) error {
		m, err := tx.GetMessage("m1")
		if err != nil || m.Body != "keep me" {
			t.Fatalf("restored message: %+v %v", m, err)
		}
		return nil
	})
}

func TestVerifyBackupRejectsDamagedOrWrongVersion(t *testing.T) {
	db := openTest(t)
	seed(t, db)
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "copy.db")
	if err := db.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBackup(ctx, dest, 2); err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("want version mismatch, got %v", err)
	}
	junk := filepath.Join(t.TempDir(), "junk.db")
	if err := os.WriteFile(junk, []byte(strings.Repeat("not a database ", 400)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBackup(ctx, junk, 0); err == nil {
		t.Fatal("a file that is not a database must fail verification")
	}
	if err := VerifyBackup(ctx, filepath.Join(t.TempDir(), "missing.db"), 0); err == nil {
		t.Fatal("a missing file must fail verification")
	}
}

func TestPendingMigrationBacksUpFirst(t *testing.T) {
	db := openTest(t)
	_, tp, _ := seed(t, db)
	ctx := context.Background()
	next := fstest.MapFS{
		"m/0001_init.sql":  {Data: []byte("-- already applied")},
		"m/0002_extra.sql": {Data: []byte("CREATE TABLE extra (x INTEGER);")},
	}
	if err := db.migrate(ctx, next, "m"); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.SchemaVersion(ctx); v != 2 {
		t.Fatalf("schema version %d, want 2", v)
	}
	matches, err := filepath.Glob(db.Path() + ".pre-v2-*.bak")
	if err != nil || len(matches) != 1 {
		t.Fatalf("pre-migration backup: %v %v", matches, err)
	}
	if err := VerifyBackup(ctx, matches[0], 1); err != nil {
		t.Fatalf("pre-migration backup must hold the old schema: %v", err)
	}
	// The embedded migrations stop at version 1, so the old copy opens
	// without migrating; its data must be there.
	copyDB, err := Open(ctx, matches[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = copyDB.View(ctx, func(tx *Tx) error {
		if _, err := tx.GetTopic(tp.ID); err != nil {
			t.Fatalf("backup lost data: %v", err)
		}
		return nil
	})
	_ = copyDB.Close()

	// Nothing pending: no second backup.
	if err := db.migrate(ctx, next, "m"); err != nil {
		t.Fatal(err)
	}
	if again, _ := filepath.Glob(db.Path() + ".pre-v*.bak"); len(again) != 1 {
		t.Fatalf("unexpected extra backups: %v", again)
	}
}

func TestFreshDatabaseIsNotBackedUp(t *testing.T) {
	db := openTest(t)
	if matches, _ := filepath.Glob(db.Path() + ".pre-v*.bak"); len(matches) != 0 {
		t.Fatalf("a new database has nothing to back up: %v", matches)
	}
}

func TestOpenExistingDoesNotCreateOrMigrate(t *testing.T) {
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenExisting(ctx, missing); err == nil {
		t.Fatal("a missing file must not be created")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenExisting created %s", missing)
	}
	db := openTest(t)
	seed(t, db)
	other, err := OpenExisting(ctx, db.Path())
	if err != nil {
		t.Fatal(err)
	}
	_ = other.Close()

	if _, err := db.sql.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (7, '0007_future.sql', ?)`, formatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(ctx, db.Path()); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("want ErrSchemaTooNew, got %v", err)
	}
	plain := filepath.Join(t.TempDir(), "plain.db")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(ctx, plain); err == nil {
		t.Fatal("an empty file is not a forum database")
	}
}

func TestNewerSchemaIsRefused(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, '0099_future.sql', ?)`, formatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("want ErrSchemaTooNew, got %v", err)
	}
	if _, err := Open(ctx, db.Path()); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Open must refuse a newer schema, got %v", err)
	}
}
