// Command forumctl works with the forum's database from the terminal: list
// topics, export a topic, back the database up and verify a backup. It is
// safe to run while the desktop application is open: it never migrates the
// database, never touches runs or rounds and only writes new files.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/app"
	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/export"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

const usage = `forumctl: terminal tools for the Agent Forum database.

Usage:
  forumctl [-data DIR] topics
  forumctl [-data DIR] export -topic ID [-format markdown|json] [-out FILE]
  forumctl [-data DIR] backup [-out FILE]
  forumctl verify FILE
  forumctl help

Commands:
  topics   list workspaces and topics with their IDs
  export   write one topic as Markdown (default) or JSON to stdout or FILE;
           sealed answers of unrevealed rounds are never included
  backup   write a verified copy of the database (default:
           DIR/backups/forum-<time>.db)
  verify   check that a backup file is a healthy forum database

-data defaults to the application's data directory.
To restore a backup: quit the application, move forum.db and its -wal and
-shm files aside, and copy the backup to forum.db.
`

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("forumctl", flag.ContinueOnError)
	global.SetOutput(stderr)
	global.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	dataDir := global.String("data", "", "data directory (default: the application's)")
	if err := global.Parse(args); err != nil {
		return 2
	}
	rest := global.Args()
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "-h" || rest[0] == "--help" {
		_, _ = fmt.Fprint(stdout, usage)
		if len(rest) == 0 {
			return 2
		}
		return 0
	}

	cmd, cmdArgs := rest[0], rest[1:]
	if cmd == "verify" {
		return verify(ctx, cmdArgs, stdout, stderr)
	}

	dir := *dataDir
	if dir == "" {
		d, err := app.DefaultDataDir()
		if err != nil {
			return fail(stderr, err)
		}
		dir = d
	}
	db, err := storage.OpenExisting(ctx, filepath.Join(dir, app.DatabaseFile))
	if err != nil {
		return fail(stderr, err)
	}
	defer func() { _ = db.Close() }()

	switch cmd {
	case "topics":
		return topics(ctx, db, stdout, stderr)
	case "export":
		return exportTopic(ctx, db, cmdArgs, stdout, stderr)
	case "backup":
		return backup(ctx, db, dir, cmdArgs, stdout, stderr)
	}
	_, _ = fmt.Fprintf(stderr, "forumctl: unknown command %q\n\n%s", cmd, usage)
	return 2
}

func topics(ctx context.Context, db *storage.DB, stdout, stderr io.Writer) int {
	err := db.View(ctx, func(tx *storage.Tx) error {
		workspaces, err := tx.ListWorkspaces()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, ws := range workspaces {
			_, _ = fmt.Fprintf(w, "# %s\n", ws.Name)
			list, err := tx.ListTopics(ws.ID)
			if err != nil {
				return err
			}
			for _, tp := range list {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", tp.ID, tp.Kind, tp.UpdatedAt.Local().Format("2006-01-02 15:04"), tp.Title)
			}
		}
		return w.Flush()
	})
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func exportTopic(ctx context.Context, db *storage.DB, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	topicID := fs.String("topic", "", "topic ID (see forumctl topics)")
	format := fs.String("format", "markdown", "markdown or json")
	out := fs.String("out", "", "file to write (default: stdout); an existing file is not overwritten")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *topicID == "" {
		_, _ = fmt.Fprintln(stderr, "forumctl export: -topic is required (see forumctl topics)")
		return 2
	}
	f, err := export.ParseFormat(*format)
	if err != nil {
		return fail(stderr, err)
	}
	var doc export.Document
	err = db.View(ctx, func(tx *storage.Tx) error {
		d, err := export.Build(tx, domain.ID(*topicID), time.Now())
		doc = d
		return err
	})
	if err != nil {
		return fail(stderr, err)
	}
	file, err := export.Render(doc, f)
	if err != nil {
		return fail(stderr, err)
	}
	if *out == "" {
		if _, err := stdout.Write(file.Content); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	if err := writeNew(*out, file.Content); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, *out)
	return 0
}

func backup(ctx context.Context, db *storage.DB, dir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "file to write (default: DIR/backups/forum-<time>.db)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dest := *out
	if dest == "" {
		dest = filepath.Join(dir, "backups", "forum-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	}
	if err := db.Backup(ctx, dest); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, dest)
	return 0
}

func verify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, "forumctl verify: expects exactly one FILE")
		return 2
	}
	if err := storage.VerifyBackup(ctx, args[0], 0); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "%s: ok\n", args[0])
	return 0
}

// writeNew writes a file that must not exist yet, readable only by the
// owner like the database itself.
func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; choose another -out", path)
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "forumctl: %v\n", err)
	return 1
}
