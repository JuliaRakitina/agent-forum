package claude

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/runner"
	"github.com/JuliaRakitina/agent-forum/internal/runners/process"
)

// Runner drives the claude binary.
type Runner struct {
	// Binary is the executable name or path; "claude" by default.
	Binary string
}

// New returns a runner for the given binary path.
func New(binary string) *Runner {
	if binary == "" {
		binary = "claude"
	}
	return &Runner{Binary: binary}
}

// Capabilities implements runner.Runner. Tool approvals are not offered yet:
// they need an MCP permission-prompt tool or the stream-json control
// protocol, both tracked for the local integration phase.
func (r *Runner) Capabilities() runner.Capabilities {
	return runner.Capabilities{
		Provider:         domain.ProviderClaude,
		Streaming:        true,
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
	res, errText, sawResult := p.Result()
	res.ExitCode = code

	switch {
	case errors.Is(runErr, process.ErrInterrupted):
		res.Partial = true
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, runner.ErrTimeout
		}
		return res, runner.ErrCancelled
	case runErr != nil && !sawResult:
		return res, fmt.Errorf("%w: %v: %s", runner.ErrProcess, runErr, tail(stderr))
	case errText != "":
		return res, fmt.Errorf("%w: %s", runner.ErrProcess, errText)
	case !sawResult:
		return res, fmt.Errorf("%w: claude exited (%d) without a result: %s", runner.ErrProcess, code, tail(stderr))
	}
	return res, nil
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return "…" + s[len(s)-500:]
	}
	return s
}
