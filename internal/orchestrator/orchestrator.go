// Package orchestrator schedules runs, enforces the round state machine and
// turns adapter output into persisted messages and events. It owns the
// worker pool and the only code path that writes run results.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"agent-forum/internal/domain"
	"agent-forum/internal/prompt"
	"agent-forum/internal/runner"
	"agent-forum/internal/storage"
)

// Config tunes the orchestrator.
type Config struct {
	// Workers is the number of runs executed concurrently.
	Workers int
	// RunTimeout bounds every run. Zero means no timeout.
	RunTimeout time.Duration
	// RunsDir is where private run directories are created.
	RunsDir string
	// KeepRunDirs leaves run directories on disk for inspection.
	KeepRunDirs bool
	// FlushInterval and FlushBytes control how streamed text is coalesced
	// into persisted chunks.
	FlushInterval time.Duration
	FlushBytes    int
	// HistoryLimit is how many visible messages a non-round run receives.
	HistoryLimit int
	// QueueSize bounds the number of queued runs.
	QueueSize int
}

func (c Config) withDefaults() Config {
	if c.Workers <= 0 {
		c.Workers = 2
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 150 * time.Millisecond
	}
	if c.FlushBytes <= 0 {
		c.FlushBytes = 2048
	}
	if c.HistoryLimit <= 0 {
		c.HistoryLimit = 50
	}
	if c.QueueSize <= 0 {
		c.QueueSize = 4096
	}
	if c.RunsDir == "" {
		c.RunsDir = filepath.Join(os.TempDir(), "agent-forum-runs")
	}
	return c
}

// Service is the orchestrator. Create it with New, call Recover and Start,
// and Stop before exit.
type Service struct {
	db      *storage.DB
	runners runner.Registry
	cfg     Config

	queue chan domain.ID

	mu     sync.Mutex
	active map[domain.ID]context.CancelFunc

	// writeMu serialises write transactions with the publishing of their
	// events, so subscribers see events in sequence order.
	writeMu sync.Mutex

	subsMu sync.Mutex
	subs   map[*Subscription]struct{}

	lifecycle context.Context
	stop      context.CancelFunc
	wg        sync.WaitGroup
	started   bool
}

// New returns a stopped service.
func New(db *storage.DB, runners runner.Registry, cfg Config) *Service {
	cfg = cfg.withDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		db:        db,
		runners:   runners,
		cfg:       cfg,
		queue:     make(chan domain.ID, cfg.QueueSize),
		active:    map[domain.ID]context.CancelFunc{},
		subs:      map[*Subscription]struct{}{},
		lifecycle: ctx,
		stop:      cancel,
	}
}

// Start launches the worker pool.
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	for i := 0; i < s.cfg.Workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
}

// Stop cancels active runs and waits for the workers to finish. Runs that
// were cancelled this way end as cancelled, not interrupted, because their
// partial result was still written.
func (s *Service) Stop() {
	s.stop()
	s.mu.Lock()
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// Recover marks runs and rounds left over from a previous process as
// interrupted. Call it before Start.
func (s *Service) Recover(ctx context.Context) error {
	return s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		runs, err := tx.ListRunsByStatus(domain.RunQueued, domain.RunRunning)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, run := range runs {
			run.Status = domain.RunInterrupted
			run.Error = "application stopped while the run was in progress"
			run.FinishedAt = now
			if err := tx.UpdateRun(run); err != nil {
				return err
			}
			emit(runStatusEvent(run))
		}
		rounds, err := tx.ListRoundsByStatus(domain.RoundRunning)
		if err != nil {
			return err
		}
		for _, round := range rounds {
			round.Status = domain.RoundInterrupted
			round.FinishedAt = now
			if err := tx.UpdateRound(round); err != nil {
				return err
			}
			emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
		}
		return nil
	})
}

// --- subscriptions --------------------------------------------------------

// Subscription delivers events after they are committed. If the subscriber
// falls behind, events are dropped and Lagged reports it; the subscriber
// then reloads from storage with ListEvents.
type Subscription struct {
	C      chan domain.Event
	svc    *Service
	mu     sync.Mutex
	lagged bool
	closed bool
}

// Subscribe registers a subscriber with the given buffer size.
func (s *Service) Subscribe(buffer int) *Subscription {
	if buffer <= 0 {
		buffer = 256
	}
	sub := &Subscription{C: make(chan domain.Event, buffer), svc: s}
	s.subsMu.Lock()
	s.subs[sub] = struct{}{}
	s.subsMu.Unlock()
	return sub
}

// Lagged reports whether events were dropped since the last call and
// resets the flag.
func (sub *Subscription) Lagged() bool {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	l := sub.lagged
	sub.lagged = false
	return l
}

// Close unregisters the subscription and closes C.
func (sub *Subscription) Close() {
	sub.svc.subsMu.Lock()
	delete(sub.svc.subs, sub)
	sub.svc.subsMu.Unlock()
	sub.mu.Lock()
	if !sub.closed {
		sub.closed = true
		close(sub.C)
	}
	sub.mu.Unlock()
}

func (s *Service) publish(events []domain.Event) {
	s.subsMu.Lock()
	subs := make([]*Subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.subsMu.Unlock()
	for _, sub := range subs {
		for _, ev := range events {
			sub.mu.Lock()
			if sub.closed {
				sub.mu.Unlock()
				break
			}
			select {
			case sub.C <- ev:
			default:
				sub.lagged = true
			}
			sub.mu.Unlock()
		}
	}
}

// commit runs fn in a write transaction, appends the events it emitted in
// the same transaction and publishes them after the commit, all under the
// write lock so that publication order equals sequence order.
func (s *Service) commit(ctx context.Context, fn func(tx *storage.Tx, emit func(domain.Event)) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var events []domain.Event
	err := s.db.Update(ctx, func(tx *storage.Tx) error {
		events = events[:0]
		emit := func(ev domain.Event) { events = append(events, ev) }
		if err := fn(tx, emit); err != nil {
			return err
		}
		for i := range events {
			if err := tx.AppendEvent(&events[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.publish(events)
	return nil
}

// --- public operations ----------------------------------------------------

// RoundSpec describes a round to start.
type RoundSpec struct {
	TopicID        domain.ID
	Kind           domain.RoundKind
	Prompt         string
	ParticipantIDs []domain.ID
	// TriggerMessageID is the human message that started the round.
	TriggerMessageID domain.ID
	AutoReveal       bool
}

// StartRound freezes the topic's inputs, creates the round and queues its
// first runs: all of them for an independent round, the first one for a
// single or joint round.
func (s *Service) StartRound(ctx context.Context, spec RoundSpec) (domain.Round, error) {
	if len(spec.ParticipantIDs) == 0 {
		return domain.Round{}, errors.New("orchestrator: round needs at least one participant")
	}
	if spec.Kind == "" {
		spec.Kind = domain.RoundSingle
	}
	if spec.Kind == domain.RoundSingle && len(spec.ParticipantIDs) > 1 {
		return domain.Round{}, errors.New("orchestrator: single round takes one participant")
	}
	var round domain.Round
	var queued []domain.ID
	err := s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		now := time.Now().UTC()
		inputs, err := s.freezeInputs(tx, spec.TopicID, spec.TriggerMessageID)
		if err != nil {
			return err
		}
		round = domain.Round{
			ID: domain.NewID(), TopicID: spec.TopicID, Kind: spec.Kind, Status: domain.RoundRunning,
			Prompt: spec.Prompt, ParticipantIDs: spec.ParticipantIDs, Inputs: inputs, AutoReveal: spec.AutoReveal, CreatedAt: now,
		}
		if err := tx.CreateRound(round); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
		first := spec.ParticipantIDs
		if spec.Kind != domain.RoundIndependent {
			first = spec.ParticipantIDs[:1]
		}
		for _, pid := range first {
			run, err := s.createRun(tx, round, pid, spec.TriggerMessageID, now)
			if err != nil {
				return err
			}
			emit(runStatusEvent(run))
			queued = append(queued, run.ID)
		}
		if err := tx.TouchTopic(spec.TopicID, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Round{}, err
	}
	return round, s.enqueue(queued...)
}

// CancelRun stops a queued or running run.
func (s *Service) CancelRun(ctx context.Context, runID domain.ID) error {
	s.mu.Lock()
	cancel, running := s.active[runID]
	s.mu.Unlock()
	if running {
		cancel()
		return nil
	}
	return s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		run, err := tx.GetRun(runID)
		if err != nil {
			return err
		}
		if run.Status != domain.RunQueued {
			return nil // already running (will be picked up above) or finished
		}
		run.Status = domain.RunCancelled
		run.FinishedAt = time.Now().UTC()
		if err := tx.UpdateRun(run); err != nil {
			return err
		}
		emit(runStatusEvent(run))
		return s.settleRound(tx, emit, run.RoundID)
	})
}

// CancelRound cancels every unfinished run of a round and marks the round
// cancelled.
func (s *Service) CancelRound(ctx context.Context, roundID domain.ID) error {
	var toCancel []domain.ID
	err := s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		round, err := tx.GetRound(roundID)
		if err != nil {
			return err
		}
		if round.Status.Terminal() {
			return fmt.Errorf("orchestrator: round is %s", round.Status)
		}
		runs, err := tx.ListRunsByRound(roundID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, run := range runs {
			switch run.Status {
			case domain.RunQueued:
				run.Status = domain.RunCancelled
				run.FinishedAt = now
				if err := tx.UpdateRun(run); err != nil {
					return err
				}
				emit(runStatusEvent(run))
			case domain.RunRunning:
				toCancel = append(toCancel, run.ID)
			}
		}
		round.Status = domain.RoundCancelled
		round.FinishedAt = now
		if err := tx.UpdateRound(round); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
		return nil
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, id := range toCancel {
		if cancel, ok := s.active[id]; ok {
			cancel()
		}
	}
	s.mu.Unlock()
	return nil
}

// RevealRound makes the sealed results of a round visible. It is allowed
// from awaiting_reveal and, explicitly, from partial.
func (s *Service) RevealRound(ctx context.Context, roundID domain.ID) error {
	return s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		round, err := tx.GetRound(roundID)
		if err != nil {
			return err
		}
		return s.reveal(tx, emit, round)
	})
}

// RetryRound queues new runs for the participants of a partial round that
// have no successful run yet.
func (s *Service) RetryRound(ctx context.Context, roundID domain.ID) error {
	var queued []domain.ID
	err := s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		round, err := tx.GetRound(roundID)
		if err != nil {
			return err
		}
		next, err := round.Status.Transition(domain.RoundRunning)
		if err != nil {
			return err
		}
		runs, err := tx.ListRunsByRound(roundID)
		if err != nil {
			return err
		}
		succeeded := map[domain.ID]bool{}
		for _, run := range runs {
			if run.Status == domain.RunSucceeded {
				succeeded[run.ParticipantID] = true
			}
		}
		now := time.Now().UTC()
		for _, pid := range round.ParticipantIDs {
			if succeeded[pid] {
				continue
			}
			run, err := s.createRun(tx, round, pid, "", now)
			if err != nil {
				return err
			}
			emit(runStatusEvent(run))
			queued = append(queued, run.ID)
			if round.Kind != domain.RoundIndependent {
				break
			}
		}
		round.Status = next
		if err := tx.UpdateRound(round); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
		return nil
	})
	if err != nil {
		return err
	}
	return s.enqueue(queued...)
}

// --- internals ------------------------------------------------------------

func (s *Service) enqueue(ids ...domain.ID) error {
	for _, id := range ids {
		select {
		case s.queue <- id:
		default:
			return errors.New("orchestrator: run queue is full")
		}
	}
	return nil
}

// freezeInputs snapshots the context revision and the visible history of a
// topic. The trigger message is left out because the round delivers it as
// the task.
func (s *Service) freezeInputs(tx *storage.Tx, topicID, trigger domain.ID) (domain.FrozenInputs, error) {
	var inputs domain.FrozenInputs
	cur, err := tx.CurrentContext(topicID)
	switch {
	case err == nil:
		inputs.ContextRevisionID = cur.ID
	case errors.Is(err, storage.ErrNotFound):
	default:
		return inputs, err
	}
	msgs, err := tx.ListMessages(storage.MessageQuery{TopicID: topicID, Limit: s.cfg.HistoryLimit})
	if err != nil {
		return inputs, err
	}
	inputs.MessageIDs = make([]domain.ID, 0, len(msgs))
	for _, m := range msgs {
		if m.ID == trigger {
			continue
		}
		inputs.MessageIDs = append(inputs.MessageIDs, m.ID)
	}
	return inputs, nil
}

func (s *Service) createRun(tx *storage.Tx, round domain.Round, participantID, trigger domain.ID, now time.Time) (domain.Run, error) {
	p, err := tx.GetParticipant(participantID)
	if err != nil {
		return domain.Run{}, err
	}
	isolation := domain.IsolationNone
	if round.Kind == domain.RoundIndependent {
		isolation = domain.IsolationSoft
	}
	run := domain.Run{
		ID: domain.NewID(), TopicID: round.TopicID, ParticipantID: p.ID, RoundID: round.ID, TriggerMessageID: trigger,
		Status: domain.RunQueued, Profile: p.Profile, Tools: p.Tools, Isolation: isolation, CreatedAt: now,
	}
	if run.Profile == "" {
		run.Profile = domain.ProfileSubscription
	}
	if run.Tools == "" {
		run.Tools = domain.ToolsNone
	}
	if err := tx.CreateRun(run); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.lifecycle.Done():
			return
		case id := <-s.queue:
			s.execute(id)
		}
	}
}

// execute runs one queued run end to end.
func (s *Service) execute(runID domain.ID) {
	ctx, cancel := context.WithCancel(s.lifecycle)
	if s.cfg.RunTimeout > 0 {
		// The orchestrator owns the timeout so every adapter, including
		// fakes, is bounded the same way.
		ctx, cancel = context.WithTimeout(ctx, s.cfg.RunTimeout)
	}
	defer cancel()
	s.mu.Lock()
	s.active[runID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.active, runID)
		s.mu.Unlock()
	}()

	prep, err := s.prepare(ctx, runID)
	if err != nil {
		if !errors.Is(err, errSkipped) {
			s.fail(runID, domain.RunFailed, err)
		}
		return
	}
	defer prep.cleanup()

	r, err := s.runners.Lookup(prep.participant.Provider)
	if err != nil {
		s.fail(runID, domain.RunFailed, err)
		return
	}

	sealed := prep.round != nil && prep.round.Kind == domain.RoundIndependent
	stream := newStreamer(s, runID, prep.run.TopicID, sealed, s.cfg.FlushInterval, s.cfg.FlushBytes)
	sink := runner.SinkFunc(func(ev runner.Event) {
		switch ev.Kind {
		case runner.EventTextDelta:
			stream.write(ev.Text)
		case runner.EventTool:
			stream.flush()
			s.emitTool(prep.run, ev)
		case runner.EventStarted, runner.EventStatus, runner.EventLog:
			// Diagnostics only; the native session ID is taken from the
			// result.
		}
	})
	res, runErr := r.Run(ctx, prep.request, sink)
	stream.flush()
	s.finish(prep, res, runErr)
}

var errSkipped = errors.New("orchestrator: run skipped")

type prepared struct {
	run         domain.Run
	participant domain.Participant
	round       *domain.Round
	request     runner.Request
	cleanup     func()
}

// prepare moves the run to running and assembles its request.
func (s *Service) prepare(ctx context.Context, runID domain.ID) (*prepared, error) {
	var prep prepared
	err := s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {
		run, err := tx.GetRun(runID)
		if err != nil {
			return err
		}
		if run.Status != domain.RunQueued {
			return errSkipped // cancelled while queued
		}
		p, err := tx.GetParticipant(run.ParticipantID)
		if err != nil {
			return err
		}
		mat := prompt.Material{Self: p, Participants: map[domain.ID]domain.Participant{}, Task: ""}
		parts, err := tx.ListParticipants(p.WorkspaceID)
		if err != nil {
			return err
		}
		for _, other := range parts {
			mat.Participants[other.ID] = other
		}
		var round *domain.Round
		if !run.RoundID.IsZero() {
			rd, err := tx.GetRound(run.RoundID)
			if err != nil {
				return err
			}
			round = &rd
			mat.Task = rd.Prompt
			mat.Independent = rd.Kind == domain.RoundIndependent
			if !rd.Inputs.ContextRevisionID.IsZero() {
				rev, err := tx.GetContextRevision(rd.Inputs.ContextRevisionID)
				if err != nil {
					return err
				}
				mat.Context = &rev
			}
			ids := rd.Inputs.MessageIDs
			if rd.Kind == domain.RoundJoint {
				// Joint rounds also see the answers given so far.
				answers, err := tx.ListMessages(storage.MessageQuery{TopicID: rd.TopicID})
				if err != nil {
					return err
				}
				seen := map[domain.ID]bool{}
				for _, id := range ids {
					seen[id] = true
				}
				for _, m := range answers {
					if m.RoundID == rd.ID && !seen[m.ID] {
						ids = append(ids, m.ID)
					}
				}
			}
			for _, id := range ids {
				m, err := tx.GetMessage(id)
				if err != nil {
					return err
				}
				mat.Messages = append(mat.Messages, m)
			}
		}
		out := prompt.Build(mat)

		workDir := filepath.Join(s.cfg.RunsDir, run.ID.String())
		if err := os.MkdirAll(workDir, 0o700); err != nil {
			return fmt.Errorf("orchestrator: run dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "prompt.md"), []byte(out.Prompt), 0o600); err != nil {
			return err
		}
		run.Status = domain.RunRunning
		run.WorkDir = workDir
		run.StartedAt = time.Now().UTC()
		if err := tx.UpdateRun(run); err != nil {
			return err
		}
		emit(runStatusEvent(run))

		prep = prepared{
			run: run, participant: p, round: round,
			request: runner.Request{
				RunID: run.ID, Instructions: out.Instructions, Prompt: out.Prompt,
				Model: p.Model, Effort: p.Effort, WorkDir: workDir,
				Profile: run.Profile, Tools: run.Tools, Isolation: run.Isolation, Timeout: s.cfg.RunTimeout,
			},
			cleanup: func() {
				if !s.cfg.KeepRunDirs {
					_ = os.RemoveAll(workDir)
				}
			},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &prep, nil
}

// finish records the result of a run and settles its round.
func (s *Service) finish(prep *prepared, res runner.Result, runErr error) {
	status := domain.RunSucceeded
	errText := ""
	switch {
	case runErr == nil:
	case errors.Is(runErr, runner.ErrCancelled):
		status, errText = domain.RunCancelled, runErr.Error()
	case errors.Is(runErr, runner.ErrTimeout):
		status, errText = domain.RunTimedOut, runErr.Error()
	default:
		status, errText = domain.RunFailed, runErr.Error()
	}
	sealed := prep.round != nil && prep.round.Kind == domain.RoundIndependent
	err := s.commit(context.Background(), func(tx *storage.Tx, emit func(domain.Event)) error {
		run, err := tx.GetRun(prep.run.ID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if res.Text != "" {
			msg := domain.Message{
				ID: domain.NewID(), TopicID: run.TopicID, AuthorKind: domain.AuthorAgent, ParticipantID: run.ParticipantID,
				Body: res.Text, RunID: run.ID, RoundID: run.RoundID, Visibility: domain.VisibilityVisible, CreatedAt: now,
			}
			if sealed {
				msg.Visibility = domain.VisibilitySealed
			}
			if status != domain.RunSucceeded {
				msg.Body = res.Text + "\n\n_[incomplete: " + string(status) + "]_"
			}
			if err := tx.CreateMessage(msg); err != nil {
				return err
			}
			run.ResultMessageID = msg.ID
			emit(messageEvent(domain.EventMessageCreated, msg))
		}
		run.Status = status
		run.Error = errText
		run.Usage = res.Usage
		run.NativeSessionID = res.NativeSessionID
		run.FinishedAt = now
		if err := tx.UpdateRun(run); err != nil {
			return err
		}
		emit(runStatusEvent(run))
		if err := tx.TouchTopic(run.TopicID, now); err != nil {
			return err
		}
		return s.settleRound(tx, emit, run.RoundID)
	})
	if err != nil {
		s.fail(prep.run.ID, domain.RunFailed, err)
	}
}

// fail records an error for a run that could not be executed.
func (s *Service) fail(runID domain.ID, status domain.RunStatus, cause error) {
	_ = s.commit(context.Background(), func(tx *storage.Tx, emit func(domain.Event)) error {
		run, err := tx.GetRun(runID)
		if err != nil {
			return err
		}
		if run.Status.Terminal() {
			return nil
		}
		run.Status = status
		run.Error = cause.Error()
		run.FinishedAt = time.Now().UTC()
		if err := tx.UpdateRun(run); err != nil {
			return err
		}
		emit(runStatusEvent(run))
		return s.settleRound(tx, emit, run.RoundID)
	})
}

// settleRound updates a round after one of its runs reached a terminal
// state. Joint rounds queue their next participant here.
func (s *Service) settleRound(tx *storage.Tx, emit func(domain.Event), roundID domain.ID) error {
	if roundID.IsZero() {
		return nil
	}
	round, err := tx.GetRound(roundID)
	if err != nil {
		return err
	}
	if round.Status != domain.RoundRunning {
		return nil
	}
	runs, err := tx.ListRunsByRound(roundID)
	if err != nil {
		return err
	}
	succeeded := map[domain.ID]bool{}
	for _, run := range runs {
		if !run.Status.Terminal() {
			return nil // still working
		}
		if run.Status == domain.RunSucceeded {
			succeeded[run.ParticipantID] = true
		}
	}
	allDone := true
	var next domain.ID
	for _, pid := range round.ParticipantIDs {
		if !succeeded[pid] {
			allDone = false
			if next.IsZero() {
				next = pid
			}
		}
	}
	now := time.Now().UTC()
	if !allDone {
		// A sequential round continues with the next participant only when
		// the last run succeeded; otherwise it stops as partial.
		lastOK := len(runs) > 0 && runs[len(runs)-1].Status == domain.RunSucceeded
		if round.Kind != domain.RoundIndependent && lastOK {
			run, err := s.createRun(tx, round, next, "", now)
			if err != nil {
				return err
			}
			emit(runStatusEvent(run))
			s.deferEnqueue(run.ID)
			return nil
		}
		round.Status = domain.RoundPartial
		if err := tx.UpdateRound(round); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
		return nil
	}
	if round.Kind == domain.RoundIndependent && !round.AutoReveal {
		round.Status = domain.RoundAwaitingReveal
		if err := tx.UpdateRound(round); err != nil {
			return err
		}
		emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
		return nil
	}
	return s.reveal(tx, emit, round)
}

// reveal flips sealed messages to visible and closes the round.
func (s *Service) reveal(tx *storage.Tx, emit func(domain.Event), round domain.Round) error {
	next, err := round.Status.Transition(domain.RoundRevealed)
	if err != nil {
		return err
	}
	msgs, err := tx.RevealRoundMessages(round.ID)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		emit(messageEvent(domain.EventMessageRevealed, m))
	}
	now := time.Now().UTC()
	round.Status = next
	round.RevealedAt = now
	round.FinishedAt = now
	if err := tx.UpdateRound(round); err != nil {
		return err
	}
	emit(domain.NewEvent(domain.EventRoundStatusChanged, round.TopicID, domain.RoundStatusPayload{RoundID: round.ID, Status: round.Status}))
	return nil
}

// deferEnqueue queues a run created inside a transaction once the
// transaction has committed. The queue is buffered, so pushing from a
// goroutine after a short delay is enough; a full queue is reported by
// marking the run failed.
func (s *Service) deferEnqueue(id domain.ID) {
	go func() {
		s.writeMu.Lock() // waits for the creating transaction to commit
		s.writeMu.Unlock()
		if err := s.enqueue(id); err != nil {
			s.fail(id, domain.RunFailed, err)
		}
	}()
}

func (s *Service) emitTool(run domain.Run, ev runner.Event) {
	_ = s.commit(context.Background(), func(tx *storage.Tx, emit func(domain.Event)) error {
		emit(domain.NewEvent(domain.EventRunToolActivity, run.TopicID, domain.RunToolActivityPayload{
			RunID: run.ID, Tool: ev.Tool, Summary: ev.Text, Status: ev.ToolStatus,
		}))
		return nil
	})
}

func runStatusEvent(run domain.Run) domain.Event {
	payload := domain.RunStatusPayload{RunID: run.ID, ParticipantID: run.ParticipantID, RoundID: run.RoundID, Status: run.Status, Error: run.Error}
	if run.Status.Terminal() {
		u := run.Usage
		payload.Usage = &u
	}
	return domain.NewEvent(domain.EventRunStatusChanged, run.TopicID, payload)
}

// messageEvent builds a message event; sealed messages are delivered
// without their body.
func messageEvent(t domain.EventType, m domain.Message) domain.Event {
	if m.Visibility == domain.VisibilitySealed {
		m.Body = ""
	}
	return domain.NewEvent(t, m.TopicID, domain.MessagePayload{Message: m})
}
