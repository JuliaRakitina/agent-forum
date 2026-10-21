// Package fake is a scripted runner for tests and for developing the UI
// without a CLI installed. It streams text in chunks with configurable
// delays and can be told to fail, hang or ignore cancellation.
package fake

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
)

// Script describes one fake run.
type Script struct {
	// Chunks are emitted as text deltas in order.
	Chunks []string
	// Delay is the pause before each chunk.
	Delay time.Duration
	// FailAfter, when non-negative, makes the run fail after that many
	// chunks with Err (or a generic error when Err is nil).
	FailAfter int
	Err       error
	// Hang makes the run block after the chunks until ctx is cancelled.
	Hang bool
	// Tool, when set, emits one tool activity event before the text.
	Tool string
	// Usage is reported in the result.
	Usage domain.Usage
}

// DefaultScript answers with a short fixed text.
func DefaultScript() Script {
	return Script{Chunks: []string{"Fake ", "answer."}, FailAfter: -1}
}

// Runner is a scripted adapter. The zero value answers with DefaultScript.
type Runner struct {
	mu      sync.Mutex
	scripts map[domain.ID]Script
	// Next is used when no script was registered for a run.
	Next Script
	// Started receives the request of every run, for assertions.
	Started chan runner.Request
	// Capabilities may be adjusted by tests; the zero value is filled by
	// New.
	Caps runner.Capabilities
}

// New returns a fake runner with sensible capabilities.
func New() *Runner {
	return &Runner{
		scripts: map[domain.ID]Script{},
		Next:    DefaultScript(),
		Started: make(chan runner.Request, 64),
		Caps: runner.Capabilities{
			Provider:         domain.ProviderFake,
			Streaming:        true,
			Cancel:           true,
			ToolRestrictions: true,
			Efforts:          []string{"low", "high"},
			Profiles:         []domain.RunProfile{domain.ProfileSubscription, domain.ProfileAPIKey},
			MaxIsolation:     domain.IsolationSoft,
		},
	}
}

// Script registers the script for a run ID.
func (r *Runner) Script(id domain.ID, s Script) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts[id] = s
}

// Capabilities implements runner.Runner.
func (r *Runner) Capabilities() runner.Capabilities { return r.Caps }

// Run implements runner.Runner.
func (r *Runner) Run(ctx context.Context, req runner.Request, sink runner.Sink) (runner.Result, error) {
	r.mu.Lock()
	s, ok := r.scripts[req.RunID]
	if !ok {
		s = r.Next
	}
	r.mu.Unlock()

	select {
	case r.Started <- req:
	default:
	}

	var out strings.Builder
	res := runner.Result{NativeSessionID: "fake-" + req.RunID.String(), Usage: s.Usage}
	sink.Emit(runner.Event{Kind: runner.EventStarted, NativeSessionID: res.NativeSessionID})
	if s.Tool != "" {
		sink.Emit(runner.Event{Kind: runner.EventTool, Tool: s.Tool, ToolStatus: "completed", Text: "fake tool call"})
	}
	for i, chunk := range s.Chunks {
		if s.FailAfter >= 0 && i == s.FailAfter {
			res.Text, res.Partial = out.String(), true
			err := s.Err
			if err == nil {
				err = errors.New("scripted failure")
			}
			return res, fmt.Errorf("%w: %w", runner.ErrProcess, err)
		}
		if err := wait(ctx, s.Delay); err != nil {
			res.Text, res.Partial = out.String(), true
			return res, err
		}
		out.WriteString(chunk)
		sink.Emit(runner.Event{Kind: runner.EventTextDelta, Text: chunk})
	}
	if s.Hang {
		<-ctx.Done()
		res.Text, res.Partial = out.String(), true
		return res, ctxError(ctx)
	}
	res.Text = out.String()
	res.StopReason = "end_turn"
	return res, nil
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		if err := ctx.Err(); err != nil {
			return ctxError(ctx)
		}
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctxError(ctx)
	case <-t.C:
		return nil
	}
}

func ctxError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return runner.ErrTimeout
	}
	return runner.ErrCancelled
}
