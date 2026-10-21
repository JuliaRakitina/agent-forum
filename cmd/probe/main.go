// Command probe runs one request through a CLI adapter and prints what the
// adapter sees. It is the tool for docs/EXPERIMENTS.md: it exercises the
// exact flags the forum will use, without the window or the database.
//
//	go run ./cmd/probe -provider claude -tools none -isolation soft "Reply with OK"
//	go run ./cmd/probe -provider codex -print-args "Reply with OK"
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
	"agent-forum/internal/runner/fake"
	"agent-forum/internal/runners/claude"
	"agent-forum/internal/runners/codex"
)

func main() { os.Exit(run()) }

func run() int {
	var (
		provider  = flag.String("provider", "claude", "claude, codex or fake")
		binary    = flag.String("binary", "", "executable to run (default: provider name on PATH)")
		model     = flag.String("model", "", "model name")
		effort    = flag.String("effort", "", "effort level")
		profile   = flag.String("profile", "subscription", "subscription or api_key")
		tools     = flag.String("tools", "none", "none, read_only or workspace_write")
		isolation = flag.String("isolation", "soft", "none, soft or strict")
		timeout   = flag.Duration("timeout", 5*time.Minute, "run timeout")
		workDir   = flag.String("workdir", "", "run directory (default: a fresh temp dir)")
		printArgs = flag.Bool("print-args", false, "print the command line and exit without running")
		resume    = flag.String("resume", "", "native session or thread id to resume")
	)
	flag.Parse()
	task := strings.Join(flag.Args(), " ")
	if task == "" {
		task = "Reply with the single word OK."
	}

	dir := *workDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "forum-probe-")
		if err != nil {
			return fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
	}
	dir, _ = filepath.Abs(dir)

	req := runner.Request{
		RunID:           domain.NewID(),
		Instructions:    "You are a participant in a local forum. Answer briefly.",
		Prompt:          task,
		Model:           *model,
		Effort:          *effort,
		WorkDir:         dir,
		Profile:         domain.RunProfile(*profile),
		Tools:           domain.ToolPolicy(*tools),
		Isolation:       domain.IsolationLevel(*isolation),
		Timeout:         *timeout,
		NativeSessionID: *resume,
	}

	var r runner.Runner
	switch domain.Provider(*provider) {
	case domain.ProviderClaude:
		r = claude.New(*binary)
		if *printArgs {
			inv, err := claude.BuildInvocation(req)
			return printInvocation("claude", inv.Args, inv.Env, inv.Stdin, err)
		}
	case domain.ProviderCodex:
		r = codex.New(*binary)
		if *printArgs {
			inv, err := codex.BuildInvocation(req)
			return printInvocation("codex", inv.Args, inv.Env, inv.Stdin, err)
		}
	case domain.ProviderFake:
		r = fake.New()
	default:
		return fatal(fmt.Errorf("unknown provider %q", *provider))
	}

	fmt.Fprintf(os.Stderr, "probe: provider=%s profile=%s tools=%s isolation=%s workdir=%s\n", *provider, *profile, *tools, *isolation, dir)
	fmt.Fprintf(os.Stderr, "probe: capabilities=%+v\n", r.Capabilities())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	start := time.Now()
	res, err := r.Run(ctx, req, runner.SinkFunc(func(ev runner.Event) {
		switch ev.Kind {
		case runner.EventTextDelta:
			fmt.Print(ev.Text)
		case runner.EventTool:
			fmt.Fprintf(os.Stderr, "\n[tool %s %s] %s\n", ev.Tool, ev.ToolStatus, ev.Text)
		case runner.EventStarted:
			fmt.Fprintf(os.Stderr, "[started session=%s]\n", ev.NativeSessionID)
		default:
			fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.Kind, ev.Text)
		}
	}))
	fmt.Println()
	fmt.Fprintf(os.Stderr, "probe: elapsed=%s exit=%d partial=%v session=%s stop=%q usage=%+v\n",
		time.Since(start).Round(time.Millisecond), res.ExitCode, res.Partial, res.NativeSessionID, res.StopReason, res.Usage)
	switch {
	case err == nil:
		fmt.Fprintln(os.Stderr, "probe: OK")
		return 0
	case errors.Is(err, runner.ErrCancelled):
		fmt.Fprintln(os.Stderr, "probe: cancelled, partial text kept")
		return 130
	default:
		fmt.Fprintf(os.Stderr, "probe: %v\n", err)
		return 1
	}
}

func printInvocation(bin string, args []string, env map[string]string, stdin string, err error) int {
	if err != nil {
		return fatal(err)
	}
	for k, v := range env {
		fmt.Printf("%s=%s ", k, v)
	}
	fmt.Print(bin)
	for _, a := range args {
		fmt.Printf(" %s", shellQuote(a))
	}
	fmt.Printf("\n--- stdin (%d bytes) ---\n%s\n", len(stdin), stdin)
	return 0
}

// shellQuote renders an argument for display only; the probe never runs a
// shell.
func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\n\"'{}") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

func fatal(err error) int {
	fmt.Fprintf(os.Stderr, "probe: %v\n", err)
	return 2
}
