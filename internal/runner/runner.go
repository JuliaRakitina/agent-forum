// Package runner defines the contract between the orchestrator and the CLI
// adapters. An adapter turns a Request into a child process (or a fake), maps
// the process's structured output onto Events and returns a Result. The
// orchestrator never sees provider-specific JSON.
package runner

import (
	"context"
	"errors"
	"time"

	"agent-forum/internal/domain"
)

// Capabilities declares what an adapter can do, so the UI can hide or
// disable what a provider does not support instead of pretending.
type Capabilities struct {
	Provider domain.Provider `json:"provider"`
	// Streaming: text arrives as deltas while the model writes. Without it,
	// the full text arrives at the end.
	Streaming bool `json:"streaming"`
	// Resume: a later run can continue the provider's native session.
	Resume bool `json:"resume"`
	// Cancel: a running process can be stopped with a partial result kept.
	Cancel bool `json:"cancel"`
	// ToolApprovals: the adapter can route tool permission prompts to the
	// application instead of auto-approving or auto-denying them.
	ToolApprovals bool `json:"toolApprovals"`
	// ToolRestrictions: the adapter can restrict the tool set per run.
	ToolRestrictions bool `json:"toolRestrictions"`
	// Efforts lists the effort levels the provider accepts; empty means the
	// provider has no effort setting.
	Efforts []string `json:"efforts,omitempty"`
	// Profiles lists the authentication profiles the adapter supports.
	Profiles []domain.RunProfile `json:"profiles"`
	// MaxIsolation is the strongest isolation level the adapter can enforce
	// on this machine. Strict is only reported after a verification on the
	// machine, never assumed.
	MaxIsolation domain.IsolationLevel `json:"maxIsolation"`
}

// Request is everything an adapter needs to start one run. The prompt is
// already assembled; the adapter only decides how to deliver it.
type Request struct {
	RunID domain.ID
	// Instructions is the role and formatting guidance for this participant.
	// Adapters pass it as a system prompt addition where the CLI supports
	// one, otherwise they prepend it to the prompt.
	Instructions string
	// Prompt is the task text plus the frozen materials, in the order the
	// model should read them.
	Prompt string
	Model  string
	Effort string
	// WorkDir is a private, existing directory for this run.
	WorkDir   string
	Profile   domain.RunProfile
	Tools     domain.ToolPolicy
	Isolation domain.IsolationLevel
	// Timeout bounds the whole run. Zero means no timeout.
	Timeout time.Duration
	// NativeSessionID resumes a previous session of the same provider when
	// non-empty and the adapter supports Resume.
	NativeSessionID string
	// Env holds extra environment variables for the child process, for
	// example an API key handle resolved by the application. Adapters never
	// read credentials themselves.
	Env map[string]string
}

// EventKind classifies adapter events.
type EventKind string

const (
	// EventStarted carries the provider's native session ID once known.
	EventStarted EventKind = "started"
	// EventTextDelta carries a piece of the answer text.
	EventTextDelta EventKind = "text_delta"
	// EventTool carries a tool activity summary (name, status, short text).
	EventTool EventKind = "tool"
	// EventStatus carries a human-readable progress note, for example an
	// API retry.
	EventStatus EventKind = "status"
	// EventLog carries a line of diagnostic output (stderr).
	EventLog EventKind = "log"
)

// Event is one observation from a running adapter.
type Event struct {
	Kind EventKind
	// Text is the delta, summary, status or log line.
	Text string
	// Tool is set for EventTool.
	Tool string
	// ToolStatus is set for EventTool: started, completed, failed, denied.
	ToolStatus string
	// NativeSessionID is set for EventStarted.
	NativeSessionID string
}

// Sink receives adapter events in order. Implementations must be safe for
// use from one goroutine at a time; the adapter never calls Emit
// concurrently.
type Sink interface {
	Emit(Event)
}

// SinkFunc adapts a function to the Sink interface.
type SinkFunc func(Event)

// Emit calls f.
func (f SinkFunc) Emit(e Event) { f(e) }

// Result is the outcome of a run.
type Result struct {
	// Text is the final answer. On cancellation or failure it holds whatever
	// was streamed before the stop.
	Text string
	// Partial reports that Text is incomplete.
	Partial         bool
	NativeSessionID string
	Usage           domain.Usage
	// StopReason is the provider's own reason string, kept for diagnostics.
	StopReason string
	ExitCode   int
}

// Runner is implemented by every adapter.
type Runner interface {
	// Capabilities reports what the adapter supports on this machine.
	Capabilities() Capabilities
	// Run executes the request, emitting events to sink, and returns when
	// the process has exited. Cancelling ctx must stop the child process;
	// Run then returns ErrCancelled together with the partial Result.
	// A context deadline produces ErrTimeout instead.
	Run(ctx context.Context, req Request, sink Sink) (Result, error)
}

// Sentinel errors returned by adapters. Wrap them with fmt.Errorf("%w") to
// add detail; the orchestrator matches with errors.Is.
var (
	ErrCancelled   = errors.New("runner: cancelled")
	ErrTimeout     = errors.New("runner: timed out")
	ErrUnsupported = errors.New("runner: unsupported request")
	ErrProcess     = errors.New("runner: process failed")
)

// Registry maps providers to adapters.
type Registry map[domain.Provider]Runner

// Lookup returns the adapter for a provider.
func (r Registry) Lookup(p domain.Provider) (Runner, error) {
	run, ok := r[p]
	if !ok {
		return nil, errors.New("runner: no adapter for provider " + string(p))
	}
	return run, nil
}
