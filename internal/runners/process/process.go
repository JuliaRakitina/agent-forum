// Package process starts CLI child processes and owns their lifecycle:
// argument passing without a shell, stdin delivery, line-oriented stdout
// reading, stderr capture, and a two-step stop (interrupt, then kill) when
// the context is cancelled.
package process

import (
	"bufio"
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
	errs   chan error
	stderr *limitedBuffer
	done   chan struct{}
	wait   sync.Once
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
func Start(ctx context.Context, spec Spec) (*Handle, error) {
	if spec.Path == "" {
		return nil, errors.New("process: empty path")
	}
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = mergeEnv(os.Environ(), spec.Env)
	setSysProcAttr(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stdout pipe: %w", err)
	}
	stderrBuf := &limitedBuffer{limit: orDefault(spec.StderrLimit, DefaultStderrLimit)}
	cmd.Stderr = stderrBuf
	var stdin io.WriteCloser
	if spec.Stdin != nil {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("process: stdin pipe: %w", err)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("process: start %s: %w", spec.Path, err)
	}

	h := &Handle{
		cmd:    cmd,
		lines:  make(chan string, 256),
		errs:   make(chan error, 1),
		stderr: stderrBuf,
		done:   make(chan struct{}),
	}

	if stdin != nil {
		go func() {
			_, _ = io.Copy(stdin, spec.Stdin)
			_ = stdin.Close()
		}()
	}

	go h.readLines(stdout, orDefault(spec.MaxLineBytes, DefaultMaxLineBytes))
	go h.supervise(ctx, orDefaultDuration(spec.GracePeriod, DefaultGracePeriod))
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

func (h *Handle) readLines(r io.Reader, maxLine int) {
	defer close(h.lines)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		h.lines <- sc.Text()
	}
	if err := sc.Err(); err != nil {
		select {
		case h.errs <- err:
		default:
		}
	}
}

func (h *Handle) supervise(ctx context.Context, grace time.Duration) {
	waitErr := make(chan error, 1)
	go func() { waitErr <- h.cmd.Wait() }()

	interrupted := false
	select {
	case err := <-waitErr:
		h.finish(err, false)
		return
	case <-ctx.Done():
		interrupted = true
		_ = interrupt(h.cmd)
	}
	select {
	case err := <-waitErr:
		h.finish(err, interrupted)
	case <-time.After(grace):
		_ = kill(h.cmd)
		h.finish(<-waitErr, interrupted)
	}
}

func (h *Handle) finish(waitErr error, interrupted bool) {
	h.wait.Do(func() {
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
		select {
		case scanErr := <-h.errs:
			if err == nil {
				err = fmt.Errorf("process: read stdout: %w", scanErr)
			}
		default:
		}
		h.result = waitResult{exitCode: code, err: err}
		close(h.done)
	})
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
