package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/app"
	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

func seedDataDir(t *testing.T) (string, domain.Topic) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, app.DatabaseFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	ws := domain.Workspace{ID: "ws", Name: "Personal", RootDir: "/tmp", CreatedAt: now}
	tp := domain.Topic{ID: "topic-1", WorkspaceID: ws.ID, Title: "Stack decision", Kind: domain.TopicDiscussion, CreatedAt: now, UpdatedAt: now}
	err = db.Update(ctx, func(tx *storage.Tx) error {
		if err := tx.CreateWorkspace(ws); err != nil {
			return err
		}
		if err := tx.CreateTopic(tp); err != nil {
			return err
		}
		return tx.CreateMessage(domain.Message{ID: "m1", TopicID: tp.ID, AuthorKind: domain.AuthorHuman, Body: "Wails v2 or v3?", Visibility: domain.VisibilityVisible, CreatedAt: now})
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir, tp
}

func call(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestTopicsExportBackupVerify(t *testing.T) {
	dir, tp := seedDataDir(t)

	code, out, errOut := call(t, "-data", dir, "topics")
	if code != 0 || !strings.Contains(out, "# Personal") || !strings.Contains(out, "topic-1") || !strings.Contains(out, "Stack decision") {
		t.Fatalf("topics: %d %q %q", code, out, errOut)
	}

	code, out, errOut = call(t, "-data", dir, "export", "-topic", string(tp.ID))
	if code != 0 || !strings.Contains(out, "# Stack decision") || !strings.Contains(out, "Wails v2 or v3?") {
		t.Fatalf("export to stdout: %d %q %q", code, out, errOut)
	}

	target := filepath.Join(t.TempDir(), "topic.json")
	code, _, errOut = call(t, "-data", dir, "export", "-topic", string(tp.ID), "-format", "json", "-out", target)
	if code != 0 {
		t.Fatalf("export to file: %d %q", code, errOut)
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(data), `"format": "agent-forum.topic/v1"`) {
		t.Fatalf("json file: %v %s", err, data)
	}
	if code, _, errOut = call(t, "-data", dir, "export", "-topic", string(tp.ID), "-out", target); code == 0 || !strings.Contains(errOut, "already exists") {
		t.Fatalf("export must not overwrite: %d %q", code, errOut)
	}

	code, out, errOut = call(t, "-data", dir, "backup")
	if code != 0 {
		t.Fatalf("backup: %d %q", code, errOut)
	}
	backupPath := strings.TrimSpace(out)
	if !strings.HasPrefix(backupPath, filepath.Join(dir, "backups")) {
		t.Fatalf("backup path %q", backupPath)
	}
	if code, out, errOut = call(t, "verify", backupPath); code != 0 || !strings.HasSuffix(strings.TrimSpace(out), ": ok") {
		t.Fatalf("verify: %d %q %q", code, out, errOut)
	}
}

func TestErrors(t *testing.T) {
	dir, _ := seedDataDir(t)
	if code, _, _ := call(t); code != 2 {
		t.Fatalf("no command: %d", code)
	}
	if code, out, _ := call(t, "help"); code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("help: %d %q", code, out)
	}
	if code, _, errOut := call(t, "-data", dir, "launch"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("unknown command: %d %q", code, errOut)
	}
	if code, _, errOut := call(t, "-data", dir, "export"); code != 2 || !strings.Contains(errOut, "-topic is required") {
		t.Fatalf("export without topic: %d %q", code, errOut)
	}
	if code, _, _ := call(t, "-data", dir, "export", "-topic", "missing"); code != 1 {
		t.Fatalf("unknown topic: %d", code)
	}
	empty := t.TempDir()
	if code, _, errOut := call(t, "-data", empty, "topics"); code != 1 || !strings.Contains(errOut, "forum.db") {
		t.Fatalf("missing database: %d %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(empty, app.DatabaseFile)); !os.IsNotExist(err) {
		t.Fatal("forumctl must not create a database")
	}
	if code, _, _ := call(t, "verify", filepath.Join(empty, "nope.db")); code != 1 {
		t.Fatalf("verify missing file: %d", code)
	}
}
