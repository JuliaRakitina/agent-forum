package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agent-forum/internal/domain"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seed(t *testing.T, db *DB) (domain.Workspace, domain.Topic, domain.Participant) {
	t.Helper()
	now := time.Now().UTC()
	ws := domain.Workspace{ID: domain.NewID(), Name: "ws", RootDir: "/tmp/ws", CreatedAt: now}
	tp := domain.Topic{ID: domain.NewID(), WorkspaceID: ws.ID, Title: "t", Kind: domain.TopicDiscussion, CreatedAt: now, UpdatedAt: now}
	p := domain.Participant{ID: domain.NewID(), WorkspaceID: ws.ID, Name: "claude", Provider: domain.ProviderClaude, Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone}
	err := db.Update(context.Background(), func(tx *Tx) error {
		if err := tx.CreateWorkspace(ws); err != nil {
			return err
		}
		if err := tx.CreateTopic(tp); err != nil {
			return err
		}
		return tx.PutParticipant(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	return ws, tp, p
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := db.SchemaVersion(ctx)
	if err != nil || v != 1 {
		t.Fatalf("schema version %d, %v", v, err)
	}
	// Reopening the same file must not re-run migrations.
	db2, err := Open(ctx, db.Path())
	if err != nil {
		t.Fatal(err)
	}
	_ = db2.Close()
}

func TestWorkspaceTopicParticipantRoundTrip(t *testing.T) {
	db := openTest(t)
	ws, tp, p := seed(t, db)
	ctx := context.Background()
	err := db.View(ctx, func(tx *Tx) error {
		got, err := tx.GetWorkspace(ws.ID)
		if err != nil || got.Name != "ws" || got.CreatedAt.IsZero() {
			return errorsJoin("workspace", err, got)
		}
		topics, err := tx.ListTopics(ws.ID)
		if err != nil || len(topics) != 1 || topics[0].ID != tp.ID {
			return errorsJoin("topics", err, topics)
		}
		parts, err := tx.ListParticipants(ws.ID)
		if err != nil || len(parts) != 1 || parts[0].ID != p.ID || !parts[0].Enabled {
			return errorsJoin("participants", err, parts)
		}
		_, err = tx.GetTopic("missing")
		if !errors.Is(err, ErrNotFound) {
			return errorsJoin("missing topic", err, nil)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Upsert keeps the ID and changes fields.
	p.Model = "sonnet"
	p.Enabled = false
	if err := db.Update(ctx, func(tx *Tx) error { return tx.PutParticipant(p) }); err != nil {
		t.Fatal(err)
	}
	_ = db.View(ctx, func(tx *Tx) error {
		got, _ := tx.GetParticipant(p.ID)
		if got.Model != "sonnet" || got.Enabled {
			t.Fatalf("upsert failed: %+v", got)
		}
		return nil
	})
}

func TestMessagesSealedAndReveal(t *testing.T) {
	db := openTest(t)
	_, tp, p := seed(t, db)
	ctx := context.Background()
	base := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	round := domain.Round{ID: domain.NewID(), TopicID: tp.ID, Kind: domain.RoundIndependent, Status: domain.RoundRunning, ParticipantIDs: []domain.ID{p.ID}, CreatedAt: base}
	msgs := []domain.Message{
		{ID: "m1", TopicID: tp.ID, AuthorKind: domain.AuthorHuman, Body: "q", Mentions: []domain.ID{p.ID}, Visibility: domain.VisibilityVisible, CreatedAt: base},
		{ID: "m2", TopicID: tp.ID, AuthorKind: domain.AuthorAgent, ParticipantID: p.ID, Body: "sealed answer", RoundID: round.ID, Visibility: domain.VisibilitySealed, CreatedAt: base.Add(time.Second)},
		{ID: "m3", TopicID: tp.ID, AuthorKind: domain.AuthorHuman, Body: "later", Visibility: domain.VisibilityVisible, CreatedAt: base.Add(2 * time.Second)},
	}
	err := db.Update(ctx, func(tx *Tx) error {
		if err := tx.CreateRound(round); err != nil {
			return err
		}
		for _, m := range msgs {
			if err := tx.CreateMessage(m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.View(ctx, func(tx *Tx) error {
		visible, _ := tx.ListMessages(MessageQuery{TopicID: tp.ID})
		if len(visible) != 2 || visible[0].ID != "m1" || visible[1].ID != "m3" {
			t.Fatalf("visible: %+v", visible)
		}
		all, _ := tx.ListMessages(MessageQuery{TopicID: tp.ID, IncludeSealed: true})
		if len(all) != 3 || len(all[1].Mentions) != 0 {
			t.Fatalf("all: %+v", all)
		}
		if all[0].Mentions[0] != p.ID {
			t.Fatalf("mentions lost: %+v", all[0])
		}
		page, _ := tx.ListMessages(MessageQuery{TopicID: tp.ID, Limit: 1})
		if len(page) != 1 || page[0].ID != "m3" {
			t.Fatalf("limit should return newest in order: %+v", page)
		}
		before, _ := tx.ListMessages(MessageQuery{TopicID: tp.ID, Before: base.Add(2 * time.Second)})
		if len(before) != 1 || before[0].ID != "m1" {
			t.Fatalf("before: %+v", before)
		}
		return nil
	})
	err = db.Update(ctx, func(tx *Tx) error {
		revealed, err := tx.RevealRoundMessages(round.ID)
		if err != nil {
			return err
		}
		if len(revealed) != 1 || revealed[0].ID != "m2" || revealed[0].Visibility != domain.VisibilityVisible {
			t.Fatalf("revealed: %+v", revealed)
		}
		round.Status = domain.RoundRevealed
		round.RevealedAt = base.Add(3 * time.Second)
		return tx.UpdateRound(round)
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.View(ctx, func(tx *Tx) error {
		visible, _ := tx.ListMessages(MessageQuery{TopicID: tp.ID})
		if len(visible) != 3 {
			t.Fatalf("after reveal: %+v", visible)
		}
		got, _ := tx.GetRound(round.ID)
		if got.Status != domain.RoundRevealed || got.RevealedAt.IsZero() || got.ParticipantIDs[0] != p.ID {
			t.Fatalf("round: %+v", got)
		}
		return nil
	})
}

func TestRunsAndOutput(t *testing.T) {
	db := openTest(t)
	_, tp, p := seed(t, db)
	ctx := context.Background()
	run := domain.Run{ID: domain.NewID(), TopicID: tp.ID, ParticipantID: p.ID, Status: domain.RunQueued, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone, Isolation: domain.IsolationSoft, CreatedAt: time.Now().UTC()}
	err := db.Update(ctx, func(tx *Tx) error {
		if err := tx.CreateRun(run); err != nil {
			return err
		}
		if err := tx.AppendRunOutput(run.ID, 0, "Hel"); err != nil {
			return err
		}
		return tx.AppendRunOutput(run.ID, 1, "lo")
	})
	if err != nil {
		t.Fatal(err)
	}
	run.Status = domain.RunSucceeded
	run.Usage = domain.Usage{InputTokens: 3, CostUSD: 0.5}
	run.NativeSessionID = "native"
	run.FinishedAt = time.Now().UTC()
	if err := db.Update(ctx, func(tx *Tx) error { return tx.UpdateRun(run) }); err != nil {
		t.Fatal(err)
	}
	_ = db.View(ctx, func(tx *Tx) error {
		got, err := tx.GetRun(run.ID)
		if err != nil || got.Status != domain.RunSucceeded || got.Usage.CostUSD != 0.5 || got.NativeSessionID != "native" || got.FinishedAt.IsZero() {
			t.Fatalf("run: %+v %v", got, err)
		}
		out, _ := tx.RunOutput(run.ID)
		if out != "Hello" {
			t.Fatalf("output %q", out)
		}
		byStatus, _ := tx.ListRunsByStatus(domain.RunSucceeded, domain.RunRunning)
		if len(byStatus) != 1 {
			t.Fatalf("by status: %+v", byStatus)
		}
		return nil
	})
	if err := db.Update(ctx, func(tx *Tx) error { return tx.UpdateRun(domain.Run{ID: "missing"}) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestContextRevisionsVersioning(t *testing.T) {
	db := openTest(t)
	_, tp, _ := seed(t, db)
	ctx := context.Background()
	r1 := &domain.ContextRevision{ID: domain.NewID(), TopicID: tp.ID, Status: domain.RevisionAccepted, AuthorKind: domain.AuthorHuman, Doc: domain.ContextDoc{Goal: "g1"}, CreatedAt: time.Now().UTC()}
	r2 := &domain.ContextRevision{ID: domain.NewID(), TopicID: tp.ID, Status: domain.RevisionProposed, AuthorKind: domain.AuthorAgent, Doc: domain.ContextDoc{Goal: "g2"}, CreatedAt: time.Now().UTC()}
	err := db.Update(ctx, func(tx *Tx) error {
		if err := tx.CreateContextRevision(r1); err != nil {
			return err
		}
		return tx.CreateContextRevision(r2)
	})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Version != 1 || r2.Version != 2 {
		t.Fatalf("versions %d %d", r1.Version, r2.Version)
	}
	_ = db.View(ctx, func(tx *Tx) error {
		cur, err := tx.CurrentContext(tp.ID)
		if err != nil || cur.ID != r1.ID || cur.Doc.Goal != "g1" {
			t.Fatalf("current: %+v %v", cur, err)
		}
		return nil
	})
	if err := db.Update(ctx, func(tx *Tx) error { return tx.SetRevisionStatus(r2.ID, domain.RevisionAccepted) }); err != nil {
		t.Fatal(err)
	}
	_ = db.View(ctx, func(tx *Tx) error {
		cur, _ := tx.CurrentContext(tp.ID)
		if cur.ID != r2.ID {
			t.Fatalf("current after accept: %+v", cur)
		}
		return nil
	})
}

func TestEventLogSequence(t *testing.T) {
	db := openTest(t)
	_, tp, _ := seed(t, db)
	ctx := context.Background()
	var seqs []int64
	err := db.Update(ctx, func(tx *Tx) error {
		for i := 0; i < 3; i++ {
			ev := domain.NewEvent(domain.EventRunOutput, tp.ID, domain.RunOutputPayload{RunID: "r", Index: i, Text: "x"})
			if err := tx.AppendEvent(&ev); err != nil {
				return err
			}
			seqs = append(seqs, ev.Seq)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 3 {
		t.Fatalf("seqs %v", seqs)
	}
	// A rolled back transaction must not leave a gap visible to readers.
	_ = db.Update(ctx, func(tx *Tx) error {
		ev := domain.NewEvent(domain.EventRunOutput, tp.ID, nil)
		_ = tx.AppendEvent(&ev)
		return errors.New("rollback")
	})
	_ = db.View(ctx, func(tx *Tx) error {
		evs, err := tx.ListEvents(1, 10)
		if err != nil || len(evs) != 2 || evs[0].Seq != 2 || evs[1].Seq != 3 || evs[0].Type != domain.EventRunOutput {
			t.Fatalf("events %+v %v", evs, err)
		}
		last, _ := tx.LastEventSeq()
		if last != 3 {
			t.Fatalf("last seq %d", last)
		}
		return nil
	})
}

func TestTimestampsSortLexically(t *testing.T) {
	a := time.Date(2026, 10, 21, 10, 0, 0, 120000000, time.UTC)
	b := time.Date(2026, 10, 21, 10, 0, 0, 123456789, time.UTC)
	fa, fb := formatTime(a), formatTime(b)
	if len(fa) != len(fb) || fa >= fb {
		t.Fatalf("timestamps must be fixed width and ordered: %q %q", fa, fb)
	}
	if got := parseTime(sql.NullString{String: fa, Valid: true}); !got.Equal(a) {
		t.Fatalf("round trip: %v != %v", got, a)
	}
}

func TestConcurrentWritersWaitInsteadOfFailing(t *testing.T) {
	db := openTest(t)
	_, tp, _ := seed(t, db)
	ctx := context.Background()
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			errs <- db.Update(ctx, func(tx *Tx) error {
				if _, err := tx.GetTopic(tp.ID); err != nil { // read first, then write
					return err
				}
				time.Sleep(100 * time.Millisecond)
				ev := domain.NewEvent(domain.EventTopicCreated, tp.ID, nil)
				return tx.AppendEvent(&ev)
			})
		}(i)
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("writer failed: %v", err)
		}
	}
}

func errorsJoin(what string, err error, _ any) error {
	if err != nil {
		return err
	}
	return errors.New(what + ": unexpected value")
}
