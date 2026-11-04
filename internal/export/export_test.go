package export

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

const (
	sealedSecret = "SEALED-ANSWER-7f3a"
	errorSecret  = "ERROR-QUOTES-ANSWER-91c2"
	roleSecret   = "ROLE-INSTRUCTIONS-55d0"
	workDir      = "/private/var/runs/run-dir-4b1e"
	nativeID     = "native-session-0c9d"
)

var base = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

// seedTopic builds a topic with a revealed joint round, an unrevealed
// independent round and a context revision.
func seedTopic(t *testing.T) (*storage.DB, domain.Topic) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ws := domain.Workspace{ID: "ws", Name: "Personal", RootDir: "/Users/someone", CreatedAt: base}
	tp := domain.Topic{ID: "topic", WorkspaceID: ws.ID, Title: "Выбор стека: Wails v2?", Kind: domain.TopicQuestion, CreatedAt: base, UpdatedAt: base}
	claude := domain.Participant{ID: "p-claude", WorkspaceID: ws.ID, Name: "Claude", Provider: domain.ProviderClaude, Model: "sonnet", Role: roleSecret, Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone}
	codex := domain.Participant{ID: "p-codex", WorkspaceID: ws.ID, Name: "Codex", Provider: domain.ProviderCodex, Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone}
	joint := domain.Round{ID: "r-joint", TopicID: tp.ID, Kind: domain.RoundJoint, Status: domain.RoundRevealed, Prompt: "Which version?", ParticipantIDs: []domain.ID{claude.ID, codex.ID}, CreatedAt: base.Add(time.Minute), RevealedAt: base.Add(3 * time.Minute)}
	blind := domain.Round{ID: "r-blind", TopicID: tp.ID, Kind: domain.RoundIndependent, Status: domain.RoundPartial, Prompt: "Isolation?", ParticipantIDs: []domain.ID{claude.ID, codex.ID}, CreatedAt: base.Add(5 * time.Minute)}
	runs := []domain.Run{
		{ID: "run-1", TopicID: tp.ID, ParticipantID: claude.ID, RoundID: joint.ID, Status: domain.RunSucceeded, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone, Isolation: domain.IsolationNone, WorkDir: workDir, NativeSessionID: nativeID, Usage: domain.Usage{OutputTokens: 12}, CreatedAt: base.Add(time.Minute)},
		{ID: "run-2", TopicID: tp.ID, ParticipantID: codex.ID, RoundID: joint.ID, Status: domain.RunFailed, Error: "rate limited", Profile: domain.ProfileSubscription, Tools: domain.ToolsNone, Isolation: domain.IsolationNone, CreatedAt: base.Add(2 * time.Minute)},
		{ID: "run-3", TopicID: tp.ID, ParticipantID: claude.ID, RoundID: blind.ID, Status: domain.RunSucceeded, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone, Isolation: domain.IsolationSoft, WorkDir: workDir, CreatedAt: base.Add(5 * time.Minute)},
		{ID: "run-4", TopicID: tp.ID, ParticipantID: codex.ID, RoundID: blind.ID, Status: domain.RunFailed, Error: errorSecret, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone, Isolation: domain.IsolationSoft, CreatedAt: base.Add(5 * time.Minute)},
	}
	msgs := []domain.Message{
		{ID: "m1", TopicID: tp.ID, AuthorKind: domain.AuthorHuman, Body: "Which version?", Mentions: []domain.ID{claude.ID}, Visibility: domain.VisibilityVisible, CreatedAt: base.Add(time.Minute)},
		{ID: "m2", TopicID: tp.ID, AuthorKind: domain.AuthorAgent, ParticipantID: claude.ID, Body: "Take **v2**.", RunID: "run-1", RoundID: joint.ID, Visibility: domain.VisibilityVisible, CreatedAt: base.Add(2 * time.Minute)},
		{ID: "m3", TopicID: tp.ID, ParentID: "m2", AuthorKind: domain.AuthorHuman, Body: "Why not v3?", Visibility: domain.VisibilityVisible, CreatedAt: base.Add(4 * time.Minute)},
		{ID: "m4", TopicID: tp.ID, AuthorKind: domain.AuthorAgent, ParticipantID: claude.ID, Body: sealedSecret, RunID: "run-3", RoundID: blind.ID, Visibility: domain.VisibilitySealed, CreatedAt: base.Add(6 * time.Minute)},
	}
	err = db.Update(ctx, func(tx *storage.Tx) error {
		if err := tx.CreateWorkspace(ws); err != nil {
			return err
		}
		if err := tx.CreateTopic(tp); err != nil {
			return err
		}
		for _, p := range []domain.Participant{claude, codex} {
			if err := tx.PutParticipant(p); err != nil {
				return err
			}
		}
		for _, r := range []domain.Round{joint, blind} {
			if err := tx.CreateRound(r); err != nil {
				return err
			}
		}
		for _, r := range runs {
			if err := tx.CreateRun(r); err != nil {
				return err
			}
		}
		for _, m := range msgs {
			if err := tx.CreateMessage(m); err != nil {
				return err
			}
		}
		rev := &domain.ContextRevision{ID: "rev1", TopicID: tp.ID, Status: domain.RevisionAccepted, AuthorKind: domain.AuthorHuman, CreatedAt: base,
			Doc: domain.ContextDoc{Goal: "Pick a shell", Constraints: []string{"macOS first"}, Decisions: []domain.Decision{{Text: "Wails v2", Reason: "stable branch"}}}}
		return tx.CreateContextRevision(rev)
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, tp
}

func build(t *testing.T, db *storage.DB, id domain.ID) Document {
	t.Helper()
	var doc Document
	err := db.View(context.Background(), func(tx *storage.Tx) error {
		d, err := Build(tx, id, base.Add(time.Hour))
		doc = d
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestExportLeavesOutSealedAndPrivateData(t *testing.T) {
	db, tp := seedTopic(t)
	doc := build(t, db, tp.ID)
	for _, f := range []Format{JSON, Markdown} {
		file, err := Render(doc, f)
		if err != nil {
			t.Fatal(err)
		}
		out := string(file.Content)
		for _, secret := range []string{sealedSecret, errorSecret, roleSecret, workDir, nativeID} {
			if strings.Contains(out, secret) {
				t.Errorf("%s export contains %q", f, secret)
			}
		}
	}
	if doc.Omitted.SealedMessages != 1 || len(doc.Messages) != 3 {
		t.Fatalf("messages %d, omitted %d", len(doc.Messages), doc.Omitted.SealedMessages)
	}
	if len(doc.Rounds) != 2 || !doc.Rounds[1].Sealed || doc.Rounds[0].Sealed {
		t.Fatalf("rounds: %+v", doc.Rounds)
	}
	if doc.Rounds[0].Runs[1].Error != "rate limited" {
		t.Fatalf("errors of revealed rounds stay: %+v", doc.Rounds[0].Runs)
	}
}

func TestExportJSONShape(t *testing.T) {
	db, tp := seedTopic(t)
	file, err := Render(build(t, db, tp.ID), JSON)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(file.Content, &back); err != nil {
		t.Fatal(err)
	}
	if back["format"] != FormatVersion {
		t.Fatalf("format %v", back["format"])
	}
	ctxDoc, ok := back["context"].(map[string]any)
	if !ok || ctxDoc["version"].(float64) != 1 {
		t.Fatalf("context %v", back["context"])
	}
	parts := back["participants"].([]any)
	if len(parts) != 2 {
		t.Fatalf("participants %v", parts)
	}
	if _, hasRole := parts[0].(map[string]any)["role"]; hasRole {
		t.Fatal("participant role must not be exported")
	}
	if file.Name != "выбор-стека-wails-v2-2026-11-04.json" {
		t.Fatalf("file name %q", file.Name)
	}
}

func TestExportMarkdown(t *testing.T) {
	db, tp := seedTopic(t)
	md := RenderMarkdown(build(t, db, tp.ID))
	for _, want := range []string{
		"# Выбор стека: Wails v2?\n",
		"question · exported 2026-11-04 11:00 UTC · agent-forum.topic/v1",
		"> 1 sealed answer from unrevealed rounds is not included.",
		"- Claude (claude, sonnet)\n- Codex (codex)",
		"## Shared context (revision 1)",
		"**Goal:** Pick a shell",
		"- Wails v2. Reason: stable branch",
		"### Human · 2026-11-04 10:01 UTC\n\nWhich version?",
		"### Claude · 2026-11-04 10:02 UTC · joint round\n\nTake **v2**.",
		"_In reply to Claude, 2026-11-04 10:02 UTC._\n\nWhy not v3?",
		"| 2026-11-04 10:05 UTC | independent | partial (sealed) | Claude: succeeded, Codex: failed |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestEmptyTopicAndUnknownTopic(t *testing.T) {
	db, tp := seedTopic(t)
	ctx := context.Background()
	empty := domain.Topic{ID: "empty", WorkspaceID: tp.WorkspaceID, Title: "!!!", Kind: domain.TopicDiscussion, CreatedAt: base, UpdatedAt: base}
	if err := db.Update(ctx, func(tx *storage.Tx) error { return tx.CreateTopic(empty) }); err != nil {
		t.Fatal(err)
	}
	doc := build(t, db, empty.ID)
	if doc.Context != nil || len(doc.Messages) != 0 || doc.Omitted.SealedMessages != 0 {
		t.Fatalf("empty topic: %+v", doc)
	}
	if md := RenderMarkdown(doc); !strings.Contains(md, "No messages.") || strings.Contains(md, "## Rounds") {
		t.Fatalf("empty markdown:\n%s", md)
	}
	if name := Filename(doc, Markdown); name != "topic-2026-11-04.md" {
		t.Fatalf("fallback name %q", name)
	}
	err := db.View(ctx, func(tx *storage.Tx) error {
		_, err := Build(tx, "missing", base)
		return err
	})
	if err == nil {
		t.Fatal("unknown topic must be an error")
	}
}

func TestParseFormat(t *testing.T) {
	cases := []struct {
		in   string
		want Format
	}{{"md", Markdown}, {"Markdown", Markdown}, {"", Markdown}, {"\tjson\n", JSON}}
	for _, c := range cases {
		if got, err := ParseFormat(c.in); err != nil || got != c.want {
			t.Errorf("%q: %v %v", c.in, got, err)
		}
	}
	if _, err := ParseFormat("pdf"); err == nil {
		t.Error("pdf must be rejected")
	}
}
