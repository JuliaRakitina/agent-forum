package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/runner"
	"github.com/JuliaRakitina/agent-forum/internal/runner/fake"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

type fixture struct {
	t    *testing.T
	db   *storage.DB
	svc  *Service
	fake *fake.Runner
	ws   domain.Workspace
	tp   domain.Topic
	a, b domain.Participant
	sub  *Subscription
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fk := fake.New()
	cfg.RunsDir = filepath.Join(t.TempDir(), "runs")
	svc := New(db, runner.Registry{domain.ProviderFake: fk}, cfg)
	now := time.Now().UTC()
	f := &fixture{t: t, db: db, svc: svc, fake: fk}
	f.ws = domain.Workspace{ID: domain.NewID(), Name: "ws", RootDir: t.TempDir(), CreatedAt: now}
	f.tp = domain.Topic{ID: domain.NewID(), WorkspaceID: f.ws.ID, Title: "t", Kind: domain.TopicDiscussion, CreatedAt: now, UpdatedAt: now}
	f.a = domain.Participant{ID: domain.NewID(), WorkspaceID: f.ws.ID, Name: "A", Provider: domain.ProviderFake, Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone}
	f.b = domain.Participant{ID: domain.NewID(), WorkspaceID: f.ws.ID, Name: "B", Provider: domain.ProviderFake, Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone}
	err = db.Update(ctx, func(tx *storage.Tx) error {
		if err := tx.CreateWorkspace(f.ws); err != nil {
			return err
		}
		if err := tx.CreateTopic(f.tp); err != nil {
			return err
		}
		if err := tx.PutParticipant(f.a); err != nil {
			return err
		}
		if err := tx.PutParticipant(f.b); err != nil {
			return err
		}
		return tx.CreateMessage(domain.Message{ID: "m-human", TopicID: f.tp.ID, AuthorKind: domain.AuthorHuman, Body: "What do you think?", Visibility: domain.VisibilityVisible, CreatedAt: now})
	})
	if err != nil {
		t.Fatal(err)
	}
	f.sub = svc.Subscribe(1024)
	svc.Start()
	t.Cleanup(svc.Stop)
	return f
}

// waitRound polls until the round reaches one of the wanted states.
func (f *fixture) waitRound(id domain.ID, want ...domain.RoundStatus) {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var round domain.Round
		_ = f.db.View(context.Background(), func(tx *storage.Tx) error {
			r, err := tx.GetRound(id)
			round = r
			return err
		})
		for _, w := range want {
			if round.Status == w {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("round %s did not reach %v", id, want)
}

func (f *fixture) runs(roundID domain.ID) []domain.Run {
	var runs []domain.Run
	_ = f.db.View(context.Background(), func(tx *storage.Tx) error {
		r, err := tx.ListRunsByRound(roundID)
		runs = r
		return err
	})
	return runs
}

func (f *fixture) messages(includeSealed bool) []domain.Message {
	var msgs []domain.Message
	_ = f.db.View(context.Background(), func(tx *storage.Tx) error {
		m, err := tx.ListMessages(storage.MessageQuery{TopicID: f.tp.ID, IncludeSealed: includeSealed})
		msgs = m
		return err
	})
	return msgs
}

// drain collects the events published so far.
func (f *fixture) drain() []domain.Event {
	var out []domain.Event
	for {
		select {
		case ev := <-f.sub.C:
			out = append(out, ev)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
}

func TestSingleRoundProducesVisibleMessageAndOrderedEvents(t *testing.T) {
	f := newFixture(t, Config{Workers: 1, FlushInterval: 10 * time.Millisecond})
	f.fake.Next = fake.Script{Chunks: []string{"Hello ", "world"}, Delay: 5 * time.Millisecond, FailAfter: -1, Usage: domain.Usage{OutputTokens: 2, CostUSD: 0.01}}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "Answer", ParticipantIDs: []domain.ID{f.a.ID}, TriggerMessageID: "m-human"})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundRevealed)
	runs := f.runs(round.ID)
	if len(runs) != 1 || runs[0].Status != domain.RunSucceeded || runs[0].Usage.CostUSD != 0.01 || runs[0].NativeSessionID == "" || runs[0].ResultMessageID.IsZero() {
		t.Fatalf("run: %+v", runs)
	}
	msgs := f.messages(false)
	if len(msgs) != 2 || msgs[1].Body != "Hello world" || msgs[1].ParticipantID != f.a.ID || msgs[1].RoundID != round.ID {
		t.Fatalf("messages: %+v", msgs)
	}
	req := <-f.fake.Started
	if strings.Contains(req.Prompt, "[Human]\nWhat do you think?") || !strings.Contains(req.Prompt, "# Task\n\nAnswer") || req.Isolation != domain.IsolationNone {
		t.Fatalf("request: %+v", req)
	}
	if _, err := filepath.Abs(req.WorkDir); err != nil || req.WorkDir == "" {
		t.Fatalf("workdir: %q", req.WorkDir)
	}

	events := f.drain()
	var seqs []int64
	var types []string
	for _, ev := range events {
		seqs = append(seqs, ev.Seq)
		types = append(types, string(ev.Type))
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Fatalf("events out of sequence: %v", seqs)
		}
	}
	joined := strings.Join(types, " ")
	for _, want := range []string{"round.status_changed run.status_changed", "run.output", "message.created run.status_changed round.status_changed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("event order missing %q: %s", want, joined)
		}
	}
	var out domain.RunOutputPayload
	for _, ev := range events {
		if ev.Type == domain.EventRunOutput {
			_ = json.Unmarshal(ev.Payload, &out)
			break
		}
	}
	if out.Sealed || out.Text == "" {
		t.Fatalf("output payload: %+v", out)
	}
}

func TestIndependentRoundSealsUntilReveal(t *testing.T) {
	f := newFixture(t, Config{Workers: 2, FlushInterval: 10 * time.Millisecond})
	f.fake.Next = fake.Script{Chunks: []string{"answer"}, FailAfter: -1}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundIndependent, Prompt: "Answer", ParticipantIDs: []domain.ID{f.a.ID, f.b.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundAwaitingReveal)
	if visible := f.messages(false); len(visible) != 1 {
		t.Fatalf("sealed messages leaked: %+v", visible)
	}
	if all := f.messages(true); len(all) != 3 {
		t.Fatalf("expected two sealed messages: %+v", all)
	}
	for i := 0; i < 2; i++ {
		req := <-f.fake.Started
		if req.Isolation != domain.IsolationSoft || !strings.Contains(req.Instructions, "independent round") {
			t.Fatalf("request: %+v", req)
		}
	}
	events := f.drain()
	for _, ev := range events {
		switch ev.Type {
		case domain.EventMessageCreated:
			var p domain.MessagePayload
			_ = json.Unmarshal(ev.Payload, &p)
			if p.Message.Body != "" || p.Message.Visibility != domain.VisibilitySealed {
				t.Fatalf("sealed message body leaked in event: %+v", p)
			}
		case domain.EventRunOutput:
			var p domain.RunOutputPayload
			_ = json.Unmarshal(ev.Payload, &p)
			if !p.Sealed || p.Text != "" {
				t.Fatalf("sealed output leaked: %+v", p)
			}
		}
	}

	if err := f.svc.RevealRound(context.Background(), round.ID); err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundRevealed)
	if visible := f.messages(false); len(visible) != 3 {
		t.Fatalf("after reveal: %+v", visible)
	}
	revealed := 0
	for _, ev := range f.drain() {
		if ev.Type == domain.EventMessageRevealed {
			var p domain.MessagePayload
			_ = json.Unmarshal(ev.Payload, &p)
			if p.Message.Body != "answer" {
				t.Fatalf("revealed event without body: %+v", p)
			}
			revealed++
		}
	}
	if revealed != 2 {
		t.Fatalf("revealed events: %d", revealed)
	}
	if err := f.svc.RevealRound(context.Background(), round.ID); err == nil {
		t.Fatal("second reveal should fail")
	}
}

func TestIndependentRoundAutoRevealAndFailureGoesPartial(t *testing.T) {
	f := newFixture(t, Config{Workers: 2})
	f.fake.Next = fake.Script{Chunks: []string{"ok"}, FailAfter: -1}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundIndependent, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID, f.b.ID}, AutoReveal: true})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundRevealed)

	// Second round: B fails, nothing is revealed, retry succeeds.
	runs := 0
	f.fake.Next = fake.Script{Chunks: []string{"partial", "more"}, FailAfter: 1}
	round2, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundIndependent, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID, f.b.ID}, AutoReveal: true})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round2.ID, domain.RoundPartial)
	for _, r := range f.runs(round2.ID) {
		runs++
		if r.Status != domain.RunFailed || !strings.Contains(r.Error, "scripted failure") {
			t.Fatalf("run should have failed: %+v", r)
		}
	}
	if runs != 2 {
		t.Fatalf("runs: %d", runs)
	}
	if visible := f.messages(false); len(visible) != 3 {
		t.Fatalf("partial results must stay sealed: %d visible", len(visible))
	}
	f.fake.Next = fake.Script{Chunks: []string{"fixed"}, FailAfter: -1}
	if err := f.svc.RetryRound(context.Background(), round2.ID); err != nil {
		t.Fatal(err)
	}
	f.waitRound(round2.ID, domain.RoundRevealed)
	if len(f.runs(round2.ID)) != 4 {
		t.Fatalf("retry should add two runs: %+v", f.runs(round2.ID))
	}
	visible := f.messages(false)
	if len(visible) != 7 { // human + 2 + 2 partial (revealed with the round) + 2 fixed
		t.Fatalf("visible after retry: %d", len(visible))
	}
}

func TestJointRoundRunsSequentiallyWithPreviousAnswers(t *testing.T) {
	f := newFixture(t, Config{Workers: 2})
	f.fake.Next = fake.Script{Chunks: []string{"first answer"}, FailAfter: -1}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundJoint, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID, f.b.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundRevealed)
	first := <-f.fake.Started
	second := <-f.fake.Started
	if first.RunID == second.RunID || strings.Contains(first.Prompt, "first answer") {
		t.Fatalf("first run saw an answer: %q", first.Prompt)
	}
	if !strings.Contains(second.Prompt, "[A]\nfirst answer") {
		t.Fatalf("second run did not see the first answer:\n%s", second.Prompt)
	}
	if len(f.messages(false)) != 3 {
		t.Fatalf("messages: %+v", f.messages(false))
	}
}

func TestRevealRefusedWhileARunIsStillWorking(t *testing.T) {
	f := newFixture(t, Config{Workers: 2})
	f.fake.Script("fail-fast", fake.Script{Chunks: []string{"half"}, FailAfter: 0})
	// A fails at once, B hangs: the round must stay running and unrevealable.
	f.fake.Next = fake.Script{Chunks: []string{"x"}, FailAfter: -1, Hang: true}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundIndependent, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID, f.b.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// Make one of the two runs fail by cancelling it; the other keeps hanging.
	first := <-f.fake.Started
	<-f.fake.Started
	if err := f.svc.CancelRun(context.Background(), first.RunID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs := f.runs(round.ID)
		if runs[0].Status == domain.RunCancelled || runs[1].Status == domain.RunCancelled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := f.svc.RevealRound(context.Background(), round.ID); err == nil {
		t.Fatal("reveal must be refused while a run is still running")
	}
	if visible := f.messages(false); len(visible) != 1 {
		t.Fatalf("partial result leaked: %+v", visible)
	}
	if err := f.svc.CancelRound(context.Background(), round.ID); err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundCancelled)
}

func TestSealedRunsPublishNoToolSummary(t *testing.T) {
	f := newFixture(t, Config{Workers: 2})
	f.fake.Next = fake.Script{Chunks: []string{"secret"}, FailAfter: -1, Tool: "Bash"}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundIndependent, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundAwaitingReveal)
	seen := false
	for _, ev := range f.drain() {
		if ev.Type == domain.EventRunToolActivity {
			var p domain.RunToolActivityPayload
			_ = json.Unmarshal(ev.Payload, &p)
			if p.Summary != "" || p.Tool != "Bash" {
				t.Fatalf("sealed tool summary leaked: %+v", p)
			}
			seen = true
		}
	}
	if !seen {
		t.Fatal("tool activity event missing")
	}
}

func TestCancelWhileQueuedBehindWorkerIsCancelledNotFailed(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	f.fake.Next = fake.Script{Chunks: []string{"x"}, FailAfter: -1, Hang: true}
	r1, _ := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID}})
	<-f.fake.Started
	r2, _ := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "p", ParticipantIDs: []domain.ID{f.b.ID}})
	queued := f.runs(r2.ID)[0]
	if queued.Status != domain.RunQueued {
		t.Fatalf("second run should be queued: %+v", queued)
	}
	if err := f.svc.CancelRun(context.Background(), queued.ID); err != nil {
		t.Fatal(err)
	}
	f.waitRound(r2.ID, domain.RoundPartial)
	if got := f.runs(r2.ID)[0]; got.Status != domain.RunCancelled {
		t.Fatalf("queued run: %+v", got)
	}
	_ = f.svc.CancelRound(context.Background(), r1.ID)
	f.waitRound(r1.ID, domain.RoundCancelled)
}

func TestCancelRunningRunAndTimeout(t *testing.T) {
	f := newFixture(t, Config{Workers: 1, RunTimeout: 200 * time.Millisecond})
	f.fake.Next = fake.Script{Chunks: []string{"partial"}, FailAfter: -1, Hang: true}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	req := <-f.fake.Started
	if err := f.svc.CancelRun(context.Background(), req.RunID); err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundPartial)
	run := f.runs(round.ID)[0]
	if run.Status != domain.RunCancelled {
		t.Fatalf("run: %+v", run)
	}
	msgs := f.messages(false)
	if len(msgs) != 2 || !strings.Contains(msgs[1].Body, "partial") || !strings.Contains(msgs[1].Body, "incomplete: cancelled") {
		t.Fatalf("partial message: %+v", msgs)
	}

	round2, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round2.ID, domain.RoundPartial)
	if run := f.runs(round2.ID)[0]; run.Status != domain.RunTimedOut {
		t.Fatalf("run: %+v", run)
	}
}

func TestCancelRoundCancelsQueuedAndRunning(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	f.fake.Next = fake.Script{Chunks: []string{"x"}, FailAfter: -1, Hang: true}
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundIndependent, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID, f.b.ID}})
	if err != nil {
		t.Fatal(err)
	}
	<-f.fake.Started // A is running, B is queued behind the single worker
	if err := f.svc.CancelRound(context.Background(), round.ID); err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundCancelled)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs := f.runs(round.ID)
		if len(runs) == 2 && runs[0].Status == domain.RunCancelled && runs[1].Status == domain.RunCancelled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runs not cancelled: %+v", f.runs(round.ID))
}

func TestRecoverMarksInterrupted(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	now := time.Now().UTC()
	round := domain.Round{ID: domain.NewID(), TopicID: f.tp.ID, Kind: domain.RoundSingle, Status: domain.RoundRunning, ParticipantIDs: []domain.ID{f.a.ID}, CreatedAt: now}
	run := domain.Run{ID: domain.NewID(), TopicID: f.tp.ID, ParticipantID: f.a.ID, RoundID: round.ID, Status: domain.RunRunning, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone, Isolation: domain.IsolationNone, CreatedAt: now}
	err := f.db.Update(context.Background(), func(tx *storage.Tx) error {
		if err := tx.CreateRound(round); err != nil {
			return err
		}
		return tx.CreateRun(run)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.runs(round.ID)[0]; got.Status != domain.RunInterrupted {
		t.Fatalf("run: %+v", got)
	}
	f.waitRound(round.ID, domain.RoundInterrupted)
	if len(f.fake.Started) != 0 {
		t.Fatal("recovered run must not be restarted")
	}
}

func TestUnknownProviderFailsRun(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	ghost := domain.Participant{ID: domain.NewID(), WorkspaceID: f.ws.ID, Name: "ghost", Provider: "nope", Mode: domain.ParticipateManual, Enabled: true}
	_ = f.db.Update(context.Background(), func(tx *storage.Tx) error { return tx.PutParticipant(ghost) })
	round, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "p", ParticipantIDs: []domain.ID{ghost.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.waitRound(round.ID, domain.RoundPartial)
	if run := f.runs(round.ID)[0]; run.Status != domain.RunFailed || !strings.Contains(run.Error, "no adapter") {
		t.Fatalf("run: %+v", run)
	}
	if _, err := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID}); err == nil {
		t.Fatal("empty participant list should fail")
	}
}

func TestSubscriptionLagIsReported(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	small := f.svc.Subscribe(1)
	defer small.Close()
	f.fake.Next = fake.Script{Chunks: []string{"a"}, FailAfter: -1}
	round, _ := f.svc.StartRound(context.Background(), RoundSpec{TopicID: f.tp.ID, Kind: domain.RoundSingle, Prompt: "p", ParticipantIDs: []domain.ID{f.a.ID}})
	f.waitRound(round.ID, domain.RoundRevealed)
	if !small.Lagged() {
		t.Fatal("expected lag on a one-slot subscription")
	}
	if small.Lagged() {
		t.Fatal("lag flag should reset")
	}
	var missing error
	_ = f.db.View(context.Background(), func(tx *storage.Tx) error {
		evs, err := tx.ListEvents(0, 100)
		if err != nil || len(evs) < 5 {
			missing = errors.New("event log should hold the full history")
		}
		return nil
	})
	if missing != nil {
		t.Fatal(missing)
	}
}
