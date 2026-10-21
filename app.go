package main

import (
	"context"
	"log"
	"sort"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-forum/internal/app"
	"agent-forum/internal/domain"
	"agent-forum/internal/orchestrator"
	"agent-forum/internal/runner"
)

// Event names used with the Wails runtime event bus.
const (
	// EventName carries one domain.Event per emission, in sequence order.
	EventName = "forum:event"
	// ResyncName tells the UI that events were dropped and it should reload
	// from Events(lastSeq).
	ResyncName = "forum:resync"
)

// App is the Wails binding surface. Every exported method becomes a
// TypeScript function in frontend/wailsjs; the methods delegate to the
// application service and never hold logic of their own.
type App struct {
	ctx context.Context
	svc *app.Service
	sub *orchestrator.Subscription
}

// NewApp creates the binding object.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	svc, err := app.Open(ctx, app.DefaultConfig())
	if err != nil {
		log.Printf("open application: %v", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.ErrorDialog, Title: "Agent Forum", Message: err.Error()})
		runtime.Quit(ctx)
		return
	}
	a.svc = svc
	a.sub = svc.Subscribe(1024)
	go a.forwardEvents()
}

func (a *App) shutdown(ctx context.Context) {
	if a.sub != nil {
		a.sub.Close()
	}
	if a.svc != nil {
		_ = a.svc.Close()
	}
}

func (a *App) forwardEvents() {
	for ev := range a.sub.C {
		if a.sub.Lagged() {
			runtime.EventsEmit(a.ctx, ResyncName)
		}
		runtime.EventsEmit(a.ctx, EventName, ev)
	}
}

// Capabilities reports what every adapter supports, one entry per provider.
func (a *App) Capabilities() []runner.Capabilities {
	caps := a.svc.Capabilities()
	out := make([]runner.Capabilities, 0, len(caps))
	for _, c := range caps {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

// EnsureWorkspace returns the first workspace, creating it if needed.
func (a *App) EnsureWorkspace() (domain.Workspace, error) { return a.svc.EnsureWorkspace(a.ctx) }

// ListWorkspaces returns every workspace.
func (a *App) ListWorkspaces() ([]domain.Workspace, error) { return a.svc.ListWorkspaces(a.ctx) }

// CreateWorkspace creates a workspace with default participants.
func (a *App) CreateWorkspace(name, rootDir string) (domain.Workspace, error) {
	return a.svc.CreateWorkspace(a.ctx, name, rootDir)
}

// ListParticipants returns the participants of a workspace.
func (a *App) ListParticipants(workspaceID string) ([]domain.Participant, error) {
	return a.svc.ListParticipants(a.ctx, domain.ID(workspaceID))
}

// SaveParticipant creates or updates a participant.
func (a *App) SaveParticipant(p domain.Participant) (domain.Participant, error) {
	return a.svc.SaveParticipant(a.ctx, p)
}

// ListTopics returns the topics of a workspace.
func (a *App) ListTopics(workspaceID string) ([]domain.Topic, error) {
	return a.svc.ListTopics(a.ctx, domain.ID(workspaceID))
}

// CreateTopic creates a topic.
func (a *App) CreateTopic(workspaceID, title string, kind domain.TopicKind) (domain.Topic, error) {
	return a.svc.CreateTopic(a.ctx, domain.ID(workspaceID), title, kind)
}

// GetThread loads a topic with its messages, runs, rounds and context.
func (a *App) GetThread(topicID string, limit int) (app.Thread, error) {
	return a.svc.GetThread(a.ctx, domain.ID(topicID), limit)
}

// PostMessage stores a human message and starts a round for the mentions.
func (a *App) PostMessage(in app.PostInput) (app.PostResult, error) {
	return a.svc.PostMessage(a.ctx, in)
}

// CancelRun stops a run.
func (a *App) CancelRun(runID string) error { return a.svc.CancelRun(a.ctx, domain.ID(runID)) }

// CancelRound stops a round.
func (a *App) CancelRound(roundID string) error { return a.svc.CancelRound(a.ctx, domain.ID(roundID)) }

// RevealRound reveals a sealed round.
func (a *App) RevealRound(roundID string) error { return a.svc.RevealRound(a.ctx, domain.ID(roundID)) }

// RetryRound queues new runs for the failed participants of a round.
func (a *App) RetryRound(roundID string) error { return a.svc.RetryRound(a.ctx, domain.ID(roundID)) }

// RunOutput returns the streamed text of a run.
func (a *App) RunOutput(runID string) (string, error) {
	return a.svc.RunOutput(a.ctx, domain.ID(runID))
}

// SaveContext stores a human-authored context revision.
func (a *App) SaveContext(topicID string, doc domain.ContextDoc) (domain.ContextRevision, error) {
	return a.svc.SaveContext(a.ctx, domain.ID(topicID), doc)
}

// ListContextRevisions returns every context revision of a topic.
func (a *App) ListContextRevisions(topicID string) ([]domain.ContextRevision, error) {
	return a.svc.ListContextRevisions(a.ctx, domain.ID(topicID))
}

// SetContextRevisionStatus accepts or discards a proposed revision.
func (a *App) SetContextRevisionStatus(revisionID string, status domain.RevisionStatus) error {
	return a.svc.SetContextRevisionStatus(a.ctx, domain.ID(revisionID), status)
}

// Events returns events after the given sequence number.
func (a *App) Events(after int64, limit int) ([]domain.Event, error) {
	return a.svc.Events(a.ctx, after, limit)
}
