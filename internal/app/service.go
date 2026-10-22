// Package app exposes the application operations the UI calls. It wires
// storage, adapters and the orchestrator together and keeps the window
// toolkit out of the core: the Wails layer only forwards calls and events.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/orchestrator"
	"github.com/JuliaRakitina/agent-forum/internal/runner"
	"github.com/JuliaRakitina/agent-forum/internal/runner/fake"
	"github.com/JuliaRakitina/agent-forum/internal/runners/claude"
	"github.com/JuliaRakitina/agent-forum/internal/runners/codex"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

// Config configures the application.
type Config struct {
	// DataDir holds the database and managed files. Empty uses the OS
	// user-config directory.
	DataDir string
	// RunsDir holds private run directories. Empty uses the OS cache dir.
	RunsDir string
	// Workers is the number of concurrent runs.
	Workers int
	// RunTimeout bounds each run.
	RunTimeout time.Duration
	// Binaries overrides the CLI executables per provider.
	Binaries map[domain.Provider]string
	// Runners replaces the default adapter registry (tests, mock mode).
	Runners runner.Registry
	// KeepRunDirs leaves run directories on disk.
	KeepRunDirs bool
}

// DefaultConfig returns the production defaults.
func DefaultConfig() Config {
	return Config{Workers: 2, RunTimeout: 15 * time.Minute}
}

// Service is the application.
type Service struct {
	cfg     Config
	db      *storage.DB
	runners runner.Registry
	orch    *orchestrator.Service
}

// Open opens the database, recovers interrupted work and starts the
// orchestrator.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.DataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("app: config dir: %w", err)
		}
		cfg.DataDir = filepath.Join(base, "agent-forum")
	}
	if cfg.RunsDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		cfg.RunsDir = filepath.Join(base, "agent-forum", "runs")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	db, err := storage.Open(ctx, filepath.Join(cfg.DataDir, "forum.db"))
	if err != nil {
		return nil, err
	}
	runners := cfg.Runners
	if runners == nil {
		runners = runner.Registry{
			domain.ProviderClaude: claude.New(cfg.Binaries[domain.ProviderClaude]),
			domain.ProviderCodex:  codex.New(cfg.Binaries[domain.ProviderCodex]),
			domain.ProviderFake:   fake.New(),
		}
	}
	orch := orchestrator.New(db, runners, orchestrator.Config{
		Workers: cfg.Workers, RunTimeout: cfg.RunTimeout, RunsDir: cfg.RunsDir, KeepRunDirs: cfg.KeepRunDirs,
	})
	if err := orch.Recover(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	orch.Start()
	return &Service{cfg: cfg, db: db, runners: runners, orch: orch}, nil
}

// Close stops the orchestrator and closes the database.
func (s *Service) Close() error {
	s.orch.Stop()
	return s.db.Close()
}

// Subscribe returns a subscription to committed events.
func (s *Service) Subscribe(buffer int) *orchestrator.Subscription {
	return s.orch.Subscribe(buffer)
}

// DataDir returns the directory holding the database.
func (s *Service) DataDir() string { return s.cfg.DataDir }

// --- capabilities ---------------------------------------------------------

// Capabilities reports every adapter's capabilities, keyed by provider.
func (s *Service) Capabilities() map[domain.Provider]runner.Capabilities {
	out := make(map[domain.Provider]runner.Capabilities, len(s.runners))
	for p, r := range s.runners {
		out[p] = r.Capabilities()
	}
	return out
}

// --- workspaces -----------------------------------------------------------

// ListWorkspaces returns every workspace.
func (s *Service) ListWorkspaces(ctx context.Context) ([]domain.Workspace, error) {
	var out []domain.Workspace
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		ws, err := tx.ListWorkspaces()
		out = ws
		return err
	})
	return nonNilSlice(out), err
}

// CreateWorkspace creates a workspace with the default participants.
func (s *Service) CreateWorkspace(ctx context.Context, name, rootDir string) (domain.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Workspace{}, errors.New("workspace name is required")
	}
	now := time.Now().UTC()
	ws := domain.Workspace{ID: domain.NewID(), Name: name, RootDir: rootDir, CreatedAt: now}
	err := s.orch.Commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		if err := tx.CreateWorkspace(ws); err != nil {
			return err
		}
		for _, p := range defaultParticipants(ws.ID) {
			if err := tx.PutParticipant(p); err != nil {
				return err
			}
			emit(domain.NewEvent(domain.EventParticipantChanged, "", domain.ParticipantPayload{Participant: p}))
		}
		return nil
	})
	return ws, err
}

// EnsureWorkspace returns the first workspace, creating one when none
// exists.
func (s *Service) EnsureWorkspace(ctx context.Context) (domain.Workspace, error) {
	list, err := s.ListWorkspaces(ctx)
	if err != nil {
		return domain.Workspace{}, err
	}
	if len(list) > 0 {
		return list[0], nil
	}
	home, _ := os.UserHomeDir()
	return s.CreateWorkspace(ctx, "Personal", home)
}

func defaultParticipants(wsID domain.ID) []domain.Participant {
	return []domain.Participant{
		{ID: domain.NewID(), WorkspaceID: wsID, Name: "Claude", Provider: domain.ProviderClaude, Role: "", Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone},
		{ID: domain.NewID(), WorkspaceID: wsID, Name: "Codex", Provider: domain.ProviderCodex, Role: "", Mode: domain.ParticipateWhenMentioned, Enabled: true, Profile: domain.ProfileSubscription, Tools: domain.ToolsNone},
	}
}

// --- participants ---------------------------------------------------------

// ListParticipants returns the participants of a workspace.
func (s *Service) ListParticipants(ctx context.Context, workspaceID domain.ID) ([]domain.Participant, error) {
	var out []domain.Participant
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		ps, err := tx.ListParticipants(workspaceID)
		out = ps
		return err
	})
	return nonNilSlice(out), err
}

// SaveParticipant creates or updates a participant after validating it
// against the adapter's capabilities. A participant without an ID is
// created and starts enabled.
func (s *Service) SaveParticipant(ctx context.Context, p domain.Participant) (domain.Participant, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, errors.New("participant name is required")
	}
	if p.WorkspaceID.IsZero() {
		return p, errors.New("participant needs a workspace")
	}
	r, err := s.runners.Lookup(p.Provider)
	if err != nil {
		return p, err
	}
	caps := r.Capabilities()
	if p.Effort != "" && !contains(caps.Efforts, p.Effort) {
		return p, fmt.Errorf("effort %q is not offered by %s (allowed: %s)", p.Effort, p.Provider, strings.Join(caps.Efforts, ", "))
	}
	if p.Profile == "" {
		p.Profile = domain.ProfileSubscription
	}
	if !containsProfile(caps.Profiles, p.Profile) {
		return p, fmt.Errorf("profile %q is not offered by %s", p.Profile, p.Provider)
	}
	if p.Tools == "" {
		p.Tools = domain.ToolsNone
	}
	if p.Mode == "" {
		p.Mode = domain.ParticipateWhenMentioned
	}
	if p.ID.IsZero() {
		// A new participant starts enabled; disabling is an explicit edit.
		p.ID = domain.NewID()
		p.Enabled = true
	}
	err = s.orch.Commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		if err := tx.PutParticipant(p); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventParticipantChanged, "", domain.ParticipantPayload{Participant: p}))
		return nil
	})
	return p, err
}

// --- topics ---------------------------------------------------------------

// ListTopics returns the topics of a workspace, most recent first.
func (s *Service) ListTopics(ctx context.Context, workspaceID domain.ID) ([]domain.Topic, error) {
	var out []domain.Topic
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		ts, err := tx.ListTopics(workspaceID)
		out = ts
		return err
	})
	return nonNilSlice(out), err
}

// CreateTopic creates a topic.
func (s *Service) CreateTopic(ctx context.Context, workspaceID domain.ID, title string, kind domain.TopicKind) (domain.Topic, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return domain.Topic{}, errors.New("topic title is required")
	}
	if kind == "" {
		kind = domain.TopicDiscussion
	}
	now := time.Now().UTC()
	tp := domain.Topic{ID: domain.NewID(), WorkspaceID: workspaceID, Title: title, Kind: kind, CreatedAt: now, UpdatedAt: now}
	err := s.orch.Commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		if _, err := tx.GetWorkspace(workspaceID); err != nil {
			return err
		}
		if err := tx.CreateTopic(tp); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventTopicCreated, tp.ID, domain.TopicPayload{Topic: tp}))
		return nil
	})
	return tp, err
}

// Thread is everything the UI needs to render a topic.
type Thread struct {
	Topic        domain.Topic            `json:"topic"`
	Messages     []domain.Message        `json:"messages"`
	Runs         []domain.Run            `json:"runs"`
	Rounds       []domain.Round          `json:"rounds"`
	Participants []domain.Participant    `json:"participants"`
	Context      *domain.ContextRevision `json:"context"`
	LastEventSeq int64                   `json:"lastEventSeq"`
}

// GetThread loads a topic with its visible messages (the newest limit of
// them), runs, rounds, participants and current context. LastEventSeq lets
// the UI subscribe without a gap.
func (s *Service) GetThread(ctx context.Context, topicID domain.ID, limit int) (Thread, error) {
	var th Thread
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		tp, err := tx.GetTopic(topicID)
		if err != nil {
			return err
		}
		th.Topic = tp
		if th.Messages, err = tx.ListMessages(storage.MessageQuery{TopicID: topicID, Limit: limit}); err != nil {
			return err
		}
		if th.Runs, err = tx.ListRunsByTopic(topicID); err != nil {
			return err
		}
		if th.Rounds, err = tx.ListRoundsByTopic(topicID); err != nil {
			return err
		}
		if th.Participants, err = tx.ListParticipants(tp.WorkspaceID); err != nil {
			return err
		}
		cur, err := tx.CurrentContext(topicID)
		switch {
		case err == nil:
			th.Context = &cur
		case !errors.Is(err, storage.ErrNotFound):
			return err
		}
		th.LastEventSeq, err = tx.LastEventSeq()
		return err
	})
	th.Messages, th.Runs, th.Rounds, th.Participants = nonNilSlice(th.Messages), nonNilSlice(th.Runs), nonNilSlice(th.Rounds), nonNilSlice(th.Participants)
	return th, err
}

// --- posting and rounds ---------------------------------------------------

// AskMode says how the mentioned participants answer a message.
type AskMode string

const (
	// AskOne: one participant answers (RoundSingle). With several mentions
	// it behaves like AskJoint.
	AskOne AskMode = "one"
	// AskJoint: participants answer in turn and see earlier answers.
	AskJoint AskMode = "joint"
	// AskIndependent: participants answer in parallel, sealed until all
	// succeed.
	AskIndependent AskMode = "independent"
)

// PostInput is a human message with its routing.
type PostInput struct {
	TopicID  domain.ID   `json:"topicId"`
	Body     string      `json:"body"`
	ParentID domain.ID   `json:"parentId,omitempty"`
	Mentions []domain.ID `json:"mentions"`
	Mode     AskMode     `json:"mode"`
	// AutoReveal reveals an independent round as soon as all runs succeed.
	AutoReveal bool `json:"autoReveal"`
}

// PostResult is the stored message and the round it started, if any.
type PostResult struct {
	Message domain.Message `json:"message"`
	Round   *domain.Round  `json:"round"`
}

// PostMessage stores a human message and starts a round for the mentioned
// participants. Participants in "always" mode are added to the mentions.
func (s *Service) PostMessage(ctx context.Context, in PostInput) (PostResult, error) {
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return PostResult{}, errors.New("message body is required")
	}
	now := time.Now().UTC()
	msg := domain.Message{
		ID: domain.NewID(), TopicID: in.TopicID, ParentID: in.ParentID, AuthorKind: domain.AuthorHuman,
		Body: body, Mentions: in.Mentions, Visibility: domain.VisibilityVisible, CreatedAt: now,
	}
	var targets []domain.ID
	err := s.orch.Commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		tp, err := tx.GetTopic(in.TopicID)
		if err != nil {
			return err
		}
		parts, err := tx.ListParticipants(tp.WorkspaceID)
		if err != nil {
			return err
		}
		byID := map[domain.ID]domain.Participant{}
		for _, p := range parts {
			byID[p.ID] = p
		}
		seen := map[domain.ID]bool{}
		for _, id := range in.Mentions {
			p, ok := byID[id]
			if !ok {
				return fmt.Errorf("unknown participant %s", id)
			}
			if !seen[id] && p.Enabled && p.Provider != domain.ProviderHuman {
				seen[id] = true
				targets = append(targets, id)
			}
		}
		for _, p := range parts {
			if p.Mode == domain.ParticipateAlways && p.Enabled && !seen[p.ID] && p.Provider != domain.ProviderHuman {
				seen[p.ID] = true
				targets = append(targets, p.ID)
			}
		}
		if err := tx.CreateMessage(msg); err != nil {
			return err
		}
		if err := tx.TouchTopic(tp.ID, now); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventMessageCreated, msg.TopicID, domain.MessagePayload{Message: msg}))
		return nil
	})
	if err != nil {
		return PostResult{}, err
	}
	res := PostResult{Message: msg}
	if len(targets) == 0 {
		return res, nil
	}
	kind := domain.RoundSingle
	switch {
	case in.Mode == AskIndependent:
		kind = domain.RoundIndependent
	case in.Mode == AskJoint || len(targets) > 1:
		kind = domain.RoundJoint
	}
	round, err := s.orch.StartRound(ctx, orchestrator.RoundSpec{
		TopicID: in.TopicID, Kind: kind, Prompt: body, ParticipantIDs: targets, TriggerMessageID: msg.ID, AutoReveal: in.AutoReveal,
	})
	if err != nil {
		return res, err
	}
	res.Round = &round
	return res, nil
}

// CancelRun stops a run.
func (s *Service) CancelRun(ctx context.Context, runID domain.ID) error {
	return s.orch.CancelRun(ctx, runID)
}

// CancelRound stops a round.
func (s *Service) CancelRound(ctx context.Context, roundID domain.ID) error {
	return s.orch.CancelRound(ctx, roundID)
}

// RevealRound reveals a sealed round.
func (s *Service) RevealRound(ctx context.Context, roundID domain.ID) error {
	return s.orch.RevealRound(ctx, roundID)
}

// RetryRound queues new runs for the failed participants of a round.
func (s *Service) RetryRound(ctx context.Context, roundID domain.ID) error {
	return s.orch.RetryRound(ctx, roundID)
}

// EvaluationPrompt is the task given to every participant of an evaluation
// round. The revealed answers reach them through the frozen discussion.
const EvaluationPrompt = "Evaluate the answers given in the previous round, one paragraph per participant: " +
	"what is right, what is wrong or missing, and which answer you would act on and why. " +
	"Be specific and do not restate the answers."

// EvaluateRound starts an independent evaluation round over a revealed
// round: every participant of the original round judges the revealed
// answers without seeing the other judgements until the reveal.
func (s *Service) EvaluateRound(ctx context.Context, roundID domain.ID, autoReveal bool) (domain.Round, error) {
	var original domain.Round
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		r, err := tx.GetRound(roundID)
		original = r
		return err
	})
	if err != nil {
		return domain.Round{}, err
	}
	if original.Status != domain.RoundRevealed {
		return domain.Round{}, fmt.Errorf("round is %s, only a revealed round can be evaluated", original.Status)
	}
	var judges []domain.ID
	err = s.db.View(ctx, func(tx *storage.Tx) error {
		for _, id := range original.ParticipantIDs {
			p, err := tx.GetParticipant(id)
			if err != nil {
				return err
			}
			if p.Enabled && p.Provider != domain.ProviderHuman {
				judges = append(judges, id)
			}
		}
		return nil
	})
	if err != nil {
		return domain.Round{}, err
	}
	if len(judges) == 0 {
		return domain.Round{}, errors.New("no enabled participant can evaluate this round")
	}
	return s.orch.StartRound(ctx, orchestrator.RoundSpec{
		TopicID: original.TopicID, Kind: domain.RoundIndependent, Prompt: EvaluationPrompt,
		ParticipantIDs: judges, AutoReveal: autoReveal,
	})
}

// RunOutput returns the streamed text of a run. For a run of an
// unrevealed independent round it returns ErrSealed.
func (s *Service) RunOutput(ctx context.Context, runID domain.ID) (string, error) {
	var out string
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		run, err := tx.GetRun(runID)
		if err != nil {
			return err
		}
		if !run.RoundID.IsZero() {
			round, err := tx.GetRound(run.RoundID)
			if err != nil {
				return err
			}
			if round.Kind == domain.RoundIndependent && round.Status != domain.RoundRevealed {
				return ErrSealed
			}
		}
		out, err = tx.RunOutput(runID)
		return err
	})
	return out, err
}

// ErrSealed is returned when output of an unrevealed round is requested.
var ErrSealed = errors.New("run output is sealed until the round is revealed")

// --- shared context -------------------------------------------------------

// SaveContext stores a human-authored, accepted revision of the context.
func (s *Service) SaveContext(ctx context.Context, topicID domain.ID, doc domain.ContextDoc) (domain.ContextRevision, error) {
	rev := domain.ContextRevision{ID: domain.NewID(), TopicID: topicID, Status: domain.RevisionAccepted, AuthorKind: domain.AuthorHuman, Doc: doc, CreatedAt: time.Now().UTC()}
	err := s.orch.Commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		if _, err := tx.GetTopic(topicID); err != nil {
			return err
		}
		if err := tx.CreateContextRevision(&rev); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventContextRevised, topicID, domain.ContextRevisedPayload{Revision: rev}))
		return nil
	})
	return rev, err
}

// ListContextRevisions returns every revision of a topic.
func (s *Service) ListContextRevisions(ctx context.Context, topicID domain.ID) ([]domain.ContextRevision, error) {
	var out []domain.ContextRevision
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		revs, err := tx.ListContextRevisions(topicID)
		out = revs
		return err
	})
	return nonNilSlice(out), err
}

// SetContextRevisionStatus accepts or discards a proposed revision.
func (s *Service) SetContextRevisionStatus(ctx context.Context, revisionID domain.ID, status domain.RevisionStatus) error {
	if status != domain.RevisionAccepted && status != domain.RevisionDiscarded {
		return fmt.Errorf("status must be accepted or discarded, got %q", status)
	}
	return s.orch.Commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		rev, err := tx.GetContextRevision(revisionID)
		if err != nil {
			return err
		}
		if err := tx.SetRevisionStatus(revisionID, status); err != nil {
			return err
		}
		rev.Status = status
		emit(domain.NewEvent(domain.EventContextRevised, rev.TopicID, domain.ContextRevisedPayload{Revision: rev}))
		return nil
	})
}

// --- events ---------------------------------------------------------------

// Events returns up to limit events after the given sequence number.
func (s *Service) Events(ctx context.Context, after int64, limit int) ([]domain.Event, error) {
	var out []domain.Event
	err := s.db.View(ctx, func(tx *storage.Tx) error {
		evs, err := tx.ListEvents(after, limit)
		out = evs
		return err
	})
	return nonNilSlice(out), err
}

// --- helpers --------------------------------------------------------------

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func containsProfile(list []domain.RunProfile, v domain.RunProfile) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
