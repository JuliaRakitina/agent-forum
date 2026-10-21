//go:build !windows

package process

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func sh(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	return p
}

func collect(h *Handle) []string {
	var out []string
	for l := range h.Lines() {
		out = append(out, l)
	}
	return out
}

func TestLinesStdinAndExitCode(t *testing.T) {
	h, err := Start(context.Background(), Spec{
		Path:  sh(t),
		Args:  []string{"-c", `read x; echo "got:$x"; echo second; echo err >&2; exit 3`},
		Stdin: strings.NewReader("hello\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := collect(h)
	code, werr := h.Wait()
	if code != 3 || werr == nil {
		t.Fatalf("want exit 3 with error, got %d %v", code, werr)
	}
	if strings.Join(lines, "|") != "got:hello|second" {
		t.Fatalf("unexpected lines %v", lines)
	}
	if !strings.Contains(h.Stderr(), "err") {
		t.Fatalf("stderr not captured: %q", h.Stderr())
	}
}

func TestCancelInterruptsThenReportsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// The script prints one line, then sleeps; SIGINT ends it.
	h, err := Start(ctx, Spec{
		Path:        sh(t),
		Args:        []string{"-c", `echo started; sleep 30; echo never`},
		GracePeriod: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := <-h.Lines()
	if first != "started" {
		t.Fatalf("unexpected first line %q", first)
	}
	start := time.Now()
	cancel()
	rest := collect(h)
	code, werr := h.Wait()
	if !errors.Is(werr, ErrInterrupted) {
		t.Fatalf("want ErrInterrupted, got code=%d err=%v", code, werr)
	}
	if len(rest) != 0 {
		t.Fatalf("process kept writing after interrupt: %v", rest)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("stop took too long: %s", time.Since(start))
	}
}

func TestCancelKillsAfterGraceWhenInterruptIgnored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h, err := Start(ctx, Spec{
		Path:        sh(t),
		Args:        []string{"-c", `trap '' INT; echo started; sleep 30`},
		GracePeriod: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-h.Lines()
	start := time.Now()
	cancel()
	collect(h)
	_, werr := h.Wait()
	if !errors.Is(werr, ErrInterrupted) {
		t.Fatalf("want ErrInterrupted, got %v", werr)
	}
	if d := time.Since(start); d < 250*time.Millisecond || d > 5*time.Second {
		t.Fatalf("kill timing off: %s", d)
	}
}

func TestEnvOverrideAndDir(t *testing.T) {
	dir := t.TempDir()
	h, err := Start(context.Background(), Spec{
		Path: sh(t),
		Args: []string{"-c", `echo "$FORUM_TEST"; pwd`},
		Dir:  dir,
		Env:  map[string]string{"FORUM_TEST": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := collect(h)
	if _, err := h.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "value" || !strings.HasSuffix(lines[1], strings.TrimPrefix(dir, "/private")) {
		t.Fatalf("unexpected lines %v", lines)
	}
}

func TestStderrIsBounded(t *testing.T) {
	h, err := Start(context.Background(), Spec{
		Path:        sh(t),
		Args:        []string{"-c", `i=0; while [ $i -lt 2000 ]; do echo "line $i of stderr noise" >&2; i=$((i+1)); done`},
		StderrLimit: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	collect(h)
	_, _ = h.Wait()
	s := h.Stderr()
	if len(s) > 1100 || !strings.Contains(s, "bytes of stderr dropped") {
		t.Fatalf("stderr not bounded: len=%d tail=%q", len(s), s[max(0, len(s)-80):])
	}
}
