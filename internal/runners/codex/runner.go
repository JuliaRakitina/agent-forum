package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
	"agent-forum/internal/runners/process"
)

// Runner drives the codex binary in exec mode.
type Runner struct {
	Binary string
}

// New returns a runner for the given binary path; "codex" by default.
func New(binary string) *Runner {
	if binary == "" {
		binary = "codex"
	}
	return &Runner{Binary: binary}
}

// Capabilities implements runner.Runner. Streaming is reported as false:
// exec emits agent messages as items, and whether an item is updated while
// the model writes has to be checked on a real installation.
func (r *Runner) Capabilities() runner.Capabilities {
	return runner.Capabilities{
		Provider:         domain.ProviderCodex,
		Streaming:        false,
		Resume:           true,
		Cancel:           true,
		ToolApprovals:    false,
		ToolRestrictions: true,
		Efforts:          Efforts,
		Profiles:         []domain.RunProfile{domain.ProfileSubscription, domain.ProfileAPIKey},
		MaxIsolation:     domain.IsolationSoft,
	}
}

// Run implements runner.Runner.
func (r *Runner) Run(ctx context.Context, req runner.Request, sink runner.Sink) (runner.Result, error) {
	inv, err := BuildInvocation(req)
	if err != nil {
		return runner.Result{}, err
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	p := NewParser()
	code, stderr, runErr := process.Drive(ctx, process.Spec{
		Path:  r.Binary,
		Args:  inv.Args,
		Dir:   req.WorkDir,
		Env:   inv.Env,
		Stdin: strings.NewReader(inv.Stdin),
	}, func(line string) {
		for _, ev := range p.Feed(line) {
			sink.Emit(ev)
		}
	})
	res, errText, finished := p.Result()
	res.ExitCode = code

	switch {
	case errors.Is(runErr, process.ErrInterrupted):
		res.Partial = true
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, runner.ErrTimeout
		}
		return res, runner.ErrCancelled
	case errText != "":
		return res, fmt.Errorf("%w: %s", runner.ErrProcess, errText)
	case finished:
		// A completed turn counts even if the process then exited with an
		// error, for example a grandchild outliving the wait delay.
		return res, nil
	case runErr != nil:
		return res, fmt.Errorf("%w: %v: %s", runner.ErrProcess, runErr, tail(stderr))
	}
	return res, fmt.Errorf("%w: codex exited (%d) before the turn completed: %s", runner.ErrProcess, code, tail(stderr))
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return "…" + s[len(s)-500:]
	}
	return s
}
