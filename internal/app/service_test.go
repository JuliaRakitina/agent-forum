package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
	"agent-forum/internal/runner/fake"
)

func openTest(t *testing.T) (*Service, *fake.Runner) {
	t.Helper()
	fk := fake.New()
	svc, err := Open(context.Background(), Config{
		DataDir: t.TempDir(), RunsDir: t.TempDir(), Workers: 2, RunTimeout: 5 * time.Second,
		Runners: runner.Registry{domain.ProviderFake: fk},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, fk
}

func TestWorkspaceBootstrapAndParticipants(t *testing.T) {
	svc, _ := openTest(t)
	ctx := context.Background()
	ws, err := svc.EnsureWorkspace(ctx)
	if err != nil || ws.Name != "Personal" {
		t.Fatalf("%+v %v", ws, err)
	}
	again, _ := svc.EnsureWorkspace(ctx)
	if again.ID != ws.ID {
		t.Fatal("EnsureWorkspace must be idempotent")
	}
	parts, _ := svc.ListParticipants(ctx, ws.ID)
	if len(parts) != 2 {
		t.Fatalf("default participants: %+v", parts)
	}
	// Validation against capabilities.
	_, err = svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "F", Provider: domain.ProviderFake, Effort: "ultra"})
	if err == nil || !strings.Contains(err.Error(), "effort") {
		t.Fatalf("want effort error, got %v", err)
	}
	_, err = svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "F", Provider: "ghost"})
	if err == nil {
		t.Fatal("unknown provider must be rejected")
	}
	p, err := svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "F", Provider: domain.ProviderFake, Effort: "high"})
	if err != nil || p.ID.IsZero() || p.Profile != domain.ProfileSubscription || p.Tools != domain.ToolsNone || p.Mode != domain.ParticipateWhenMentioned {
		t.Fatalf("%+v %v", p, err)
	}
	caps := svc.Capabilities()
	if caps[domain.ProviderFake].Provider != domain.ProviderFake {
		t.Fatalf("capabilities: %+v", caps)
	}
}

func TestPostMessageStartsRoundAndThreadReflectsIt(t *testing.T) {
	svc, fk := openTest(t)
	ctx := context.Background()
	ws, _ := svc.EnsureWorkspace(ctx)
	a, _ := svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "A", Provider: domain.ProviderFake})
	b, _ := svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "B", Provider: domain.ProviderFake, Mode: domain.ParticipateAlways})
	tp, err := svc.CreateTopic(ctx, ws.ID, "Design", "")
	if err != nil || tp.Kind != domain.TopicDiscussion {
		t.Fatalf("%+v %v", tp, err)
	}
	fk.Next = fake.Script{Chunks: []string{"reply"}, FailAfter: -1}
	sub := svc.Subscribe(256)
	defer sub.Close()

	res, err := svc.PostMessage(ctx, PostInput{TopicID: tp.ID, Body: "  Hello @A  ", Mentions: []domain.ID{a.ID}, Mode: AskOne})
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Body != "Hello @A" || res.Round == nil || res.Round.Kind != domain.RoundJoint || len(res.Round.ParticipantIDs) != 2 {
		t.Fatalf("B is in always mode, expected a joint round with two participants: %+v", res)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		th, err := svc.GetThread(ctx, tp.ID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(th.Messages) == 3 && len(th.Rounds) == 1 && th.Rounds[0].Status == domain.RoundRevealed {
			if th.Messages[1].ParticipantID != a.ID || th.Messages[2].ParticipantID != b.ID {
				t.Fatalf("joint order: %+v", th.Messages)
			}
			if len(th.Runs) != 2 || th.LastEventSeq == 0 || len(th.Participants) != 4 {
				t.Fatalf("thread: %+v", th)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("round did not finish: %+v", th)
		}
		time.Sleep(10 * time.Millisecond)
	}
	events, _ := svc.Events(ctx, 0, 100)
	if len(events) == 0 || events[0].Type != domain.EventParticipantChanged {
		t.Fatalf("events: %+v", events)
	}
	if _, err := svc.RunOutput(ctx, "missing-run"); err == nil {
		t.Fatal("unknown run must be an error")
	}

	// A message without mentions starts no round (B in always mode still answers).
	res2, err := svc.PostMessage(ctx, PostInput{TopicID: tp.ID, Body: "note", Mode: AskOne})
	if err != nil || res2.Round == nil || res2.Round.Kind != domain.RoundSingle || res2.Round.ParticipantIDs[0] != b.ID {
		t.Fatalf("%+v %v", res2, err)
	}
	if _, err := svc.PostMessage(ctx, PostInput{TopicID: tp.ID, Body: "   "}); err == nil {
		t.Fatal("empty body must be rejected")
	}
	if _, err := svc.PostMessage(ctx, PostInput{TopicID: tp.ID, Body: "x", Mentions: []domain.ID{"nope"}}); err == nil {
		t.Fatal("unknown mention must be rejected")
	}
}

func TestSealedOutputAndContext(t *testing.T) {
	svc, fk := openTest(t)
	ctx := context.Background()
	ws, _ := svc.EnsureWorkspace(ctx)
	a, _ := svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "A", Provider: domain.ProviderFake})
	b, _ := svc.SaveParticipant(ctx, domain.Participant{WorkspaceID: ws.ID, Name: "B", Provider: domain.ProviderFake})
	tp, _ := svc.CreateTopic(ctx, ws.ID, "Design", domain.TopicQuestion)
	rev, err := svc.SaveContext(ctx, tp.ID, domain.ContextDoc{Goal: "decide"})
	if err != nil || rev.Version != 1 {
		t.Fatalf("%+v %v", rev, err)
	}
	fk.Next = fake.Script{Chunks: []string{"secret"}, FailAfter: -1}
	res, err := svc.PostMessage(ctx, PostInput{TopicID: tp.ID, Body: "q", Mentions: []domain.ID{a.ID, b.ID}, Mode: AskIndependent})
	if err != nil || res.Round.Kind != domain.RoundIndependent {
		t.Fatalf("%+v %v", res, err)
	}
	var runID domain.ID
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		th, _ := svc.GetThread(ctx, tp.ID, 50)
		if len(th.Rounds) == 1 && th.Rounds[0].Status == domain.RoundAwaitingReveal {
			runID = th.Runs[0].ID
			if len(th.Messages) != 1 || th.Context == nil || th.Context.ID != rev.ID {
				t.Fatalf("thread leaks sealed messages or lacks context: %+v", th)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if runID.IsZero() {
		t.Fatal("round did not reach awaiting_reveal")
	}
	if _, err := svc.RunOutput(ctx, runID); !errors.Is(err, ErrSealed) {
		t.Fatalf("want ErrSealed, got %v", err)
	}
	req := <-fk.Started
	if !strings.Contains(req.Prompt, "Goal: decide") {
		t.Fatalf("frozen context missing from prompt:\n%s", req.Prompt)
	}
	if err := svc.RevealRound(ctx, res.Round.ID); err != nil {
		t.Fatal(err)
	}
	out, err := svc.RunOutput(ctx, runID)
	if err != nil || out != "secret" {
		t.Fatalf("%q %v", out, err)
	}
	fk.Next = fake.Script{Chunks: []string{"verdict"}, FailAfter: -1}
	eval, err := svc.EvaluateRound(ctx, res.Round.ID, true)
	if err != nil || eval.Kind != domain.RoundIndependent || len(eval.ParticipantIDs) != 2 || eval.Prompt != EvaluationPrompt {
		t.Fatalf("%+v %v", eval, err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		th, _ := svc.GetThread(ctx, tp.ID, 50)
		if len(th.Rounds) == 2 && th.Rounds[1].Status == domain.RoundRevealed {
			if len(th.Messages) != 5 {
				t.Fatalf("evaluation messages: %d", len(th.Messages))
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		req := <-fk.Started
		if !strings.Contains(req.Prompt, "secret") {
			t.Fatalf("evaluation prompt lacks the revealed answers:\n%s", req.Prompt)
		}
	}
	if _, err := svc.EvaluateRound(ctx, res.Round.ID, true); err == nil {
		t.Fatal("evaluation must require a revealed round")
	}
	if err := svc.SetContextRevisionStatus(ctx, rev.ID, "weird"); err == nil {
		t.Fatal("invalid status must be rejected")
	}
	revs, _ := svc.ListContextRevisions(ctx, tp.ID)
	if len(revs) != 1 {
		t.Fatalf("revisions: %+v", revs)
	}
}
