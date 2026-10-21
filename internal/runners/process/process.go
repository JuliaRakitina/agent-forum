// Package process starts CLI child processes and owns their lifecycle:
// argument passing without a shell, stdin delivery, line-oriented stdout
// reading, stderr capture, and a two-step stop (interrupt, then kill) when
// the context is cancelled.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Spec describes a child process.
type Spec struct {
	Path string
	Args []string
	Dir  string
	// Env is added to the current environment. Keys override inherited
	// values.
	Env map[string]string
	// Stdin is written to the process and then closed. Nil closes stdin at
	// once.
	Stdin io.Reader
	// GracePeriod is how long to wait after the interrupt signal before the
	// process is killed. Zero uses DefaultGracePeriod.
	GracePeriod time.Duration
	// MaxLineBytes bounds a single stdout line. Zero uses DefaultMaxLineBytes.
	MaxLineBytes int
	// StderrLimit bounds the captured stderr. Zero uses DefaultStderrLimit.
	StderrLimit int
}

// Defaults for Spec.
const (
	DefaultGracePeriod  = 5 * time.Second
	DefaultMaxLineBytes = 16 << 20
	DefaultStderrLimit  = 256 << 10
)

// Handle is a started process.
type Handle struct {
	cmd    *exec.Cmd
	lines  chan string
	stderr *limitedBuffer
	done   chan struct{}
	result waitResult
}

type waitResult struct {
	exitCode int
	err      error
}

// ErrInterrupted is returned by Wait when the process was stopped because
// the context was cancelled.
var ErrInterrupted = errors.New("process: interrupted")

// Start launches the process. The returned handle delivers stdout lines on
// Lines and exits when ctx is cancelled: first an interrupt signal (SIGINT
// on Unix), then a kill after the grace period.
//
// Stdout is consumed by an io.Writer that os/exec copies into, so Wait
// waits for the copy to finish and the pipe is never closed under a
// reader. WaitDelay bounds that wait when a grandchild keeps the pipe open.
func Start(ctx context.Context, spec Spec) (*Handle, error) {
	if spec.Path == "" {
		return nil, errors.New("process: empty path")
	}
	grace := orDefaultDuration(spec.GracePeriod, DefaultGracePeriod)
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = mergeEnv(os.Environ(), spec.Env)
	cmd.WaitDelay = grace
	setSysProcAttr(cmd)

	h := &Handle{
		cmd:    cmd,
		lines:  make(chan string, 256),
		stderr: &limitedBuffer{limit: orDefault(spec.StderrLimit, DefaultStderrLimit)},
		done:   make(chan struct{}),
	}
	splitter := &lineSplitter{out: h.lines, maxLine: orDefault(spec.MaxLineBytes, DefaultMaxLineBytes)}
	cmd.Stdout = splitter
	cmd.Stderr = h.stderr
	if spec.Stdin != nil {
		cmd.Stdin = spec.Stdin
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("process: start %s: %w", spec.Path, err)
	}
	go h.supervise(ctx, grace, splitter)
	return h, nil
}

// Lines delivers stdout line by line without the trailing newline. The
// channel is closed when stdout reaches EOF.
func (h *Handle) Lines() <-chan string { return h.lines }

// Stderr returns the captured stderr so far, truncated at the limit.
func (h *Handle) Stderr() string { return h.stderr.String() }

// Wait blocks until the process has exited and returns its exit code. The
// error is ErrInterrupted when the stop was caused by context cancellation,
// an *exec.ExitError-wrapping error for a non-zero exit, or nil.
func (h *Handle) Wait() (int, error) {
	<-h.done
	return h.result.exitCode, h.result.err
}

// PID returns the process ID.
func (h *Handle) PID() int { return h.cmd.Process.Pid }

func (h *Handle) supervise(ctx context.Context, grace time.Duration, splitter *lineSplitter) {
	waitErr := make(chan error, 1)
	go func() { waitErr <- h.cmd.Wait() }()

	interrupted := false
	select {
	case err := <-waitErr:
		h.finish(err, false, splitter)
		return
	case <-ctx.Done():
		interrupted = true
		_ = interrupt(h.cmd)
	}
	select {
	case err := <-waitErr:
		h.finish(err, interrupted, splitter)
	case <-time.After(grace):
		_ = kill(h.cmd)
		h.finish(<-waitErr, interrupted, splitter)
	}
}

// finish runs once Wait has returned, so no more stdout writes can happen.
func (h *Handle) finish(waitErr error, interrupted bool, splitter *lineSplitter) {
	splitter.flush()
	close(h.lines)
	code := 0
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		code = exitErr.ExitCode()
	}
	var err error
	switch {
	case interrupted:
		err = ErrInterrupted
	case waitErr != nil:
		err = fmt.Errorf("process: exit code %d: %w", code, waitErr)
	}
	h.result = waitResult{exitCode: code, err: err}
	close(h.done)
}

// lineSplitter is the stdout writer: it cuts the byte stream into lines
// and sends them on a channel. Writes come from the goroutine os/exec uses
// to copy the pipe, one at a time.
type lineSplitter struct {
	out     chan string
	maxLine int
	buf     []byte
}

func (ls *lineSplitter) Write(p []byte) (int, error) {
	ls.buf = append(ls.buf, p...)
	for {
		i := bytesIndexByte(ls.buf, '\n')
		if i < 0 {
			break
		}
		ls.emit(ls.buf[:i])
		ls.buf = ls.buf[i+1:]
	}
	if len(ls.buf) > ls.maxLine {
		// A line longer than the limit is delivered in pieces rather than
		// growing memory without bound.
		ls.emit(ls.buf)
		ls.buf = ls.buf[:0]
	}
	return len(p), nil
}

func (ls *lineSplitter) emit(line []byte) {
	line = trimCR(line)
	ls.out <- string(line)
}

// flush delivers a trailing line without a newline.
func (ls *lineSplitter) flush() {
	if len(ls.buf) > 0 {
		ls.emit(ls.buf)
		ls.buf = nil
	}
}

func bytesIndexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func trimCR(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\r' {
		return b[:n-1]
	}
	return b
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(extra))
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if _, override := extra[key]; override {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func orDefaultDuration(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

// limitedBuffer keeps the first limit bytes written to it and counts the
// rest, so a chatty stderr cannot grow memory without bound.
type limitedBuffer struct {
	mu      sync.Mutex
	limit   int
	buf     strings.Builder
	dropped int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := b.limit - b.buf.Len()
	if room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
			b.dropped += len(p) - room
		} else {
			b.buf.Write(p)
		}
	} else {
		b.dropped += len(p)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dropped > 0 {
		return fmt.Sprintf("%s\n[... %d bytes of stderr dropped]", b.buf.String(), b.dropped)
	}
	return b.buf.String()
}

// Drive starts the process described by spec, passes every stdout line to
// onLine in order and waits for the exit. It returns the exit code, the
// captured stderr and the Wait error. onLine runs on the caller's goroutine.
func Drive(ctx context.Context, spec Spec, onLine func(line string)) (int, string, error) {
	h, err := Start(ctx, spec)
	if err != nil {
		return -1, "", err
	}
	for line := range h.Lines() {
		onLine(line)
	}
	code, err := h.Wait()
	return code, h.Stderr(), err
}
