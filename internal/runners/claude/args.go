// Package claude adapts the Claude Code CLI (`claude -p`) to the runner
// contract. Flags follow the CLI reference at
// https://code.claude.com/docs/en/cli-reference (checked 2026-10-20); the
// mapping from profiles and policies to flags is documented in
// docs/CONTRACT.md and must be re-verified on the target machine with
// docs/EXPERIMENTS.md.
package claude

import (
	"encoding/json"
	"fmt"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
)

// Efforts lists the effort levels the CLI accepts. Which ones a given model
// supports is decided by the CLI, so the UI treats this as the superset.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// Invocation is a fully built command line: arguments, environment and the
// text to deliver on stdin.
type Invocation struct {
	Args  []string
	Env   map[string]string
	Stdin string
}

// MaxArgBytes is the size above which instructions are written to a file in
// the run directory instead of being passed as an argument.
const MaxArgBytes = 32 << 10

// BuildInvocation maps a request to CLI flags. It never interpolates user
// text into a shell: everything is an argv element or stdin.
//
// Profile handling:
//   - api_key: `--bare`. The CLI then loads no hooks, skills, MCP servers,
//     CLAUDE.md or auto memory and reads no OAuth credentials, so
//     ANTHROPIC_API_KEY must be present in the environment.
//   - subscription: no `--bare` (bare mode ignores the stored login), so the
//     adapter disables the same sources one by one: --strict-mcp-config,
//     --setting-sources project (the run directory has no project settings,
//     so nothing is loaded), CLAUDE_CODE_DISABLE_AUTO_MEMORY=1, and
//     --permission-prompts none so that nothing waits for an answer.
func BuildInvocation(req runner.Request) (Invocation, error) {
	inv := Invocation{Env: map[string]string{}}
	for k, v := range req.Env {
		inv.Env[k] = v
	}
	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages"}

	switch req.Profile {
	case domain.ProfileAPIKey:
		args = append(args, "--bare")
	case domain.ProfileSubscription, "":
		args = append(args, "--strict-mcp-config", "--setting-sources", "project")
		inv.Env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] = "1"
	default:
		return inv, fmt.Errorf("%w: profile %q", runner.ErrUnsupported, req.Profile)
	}
	args = append(args, "--permission-prompts", "none")

	switch req.Isolation {
	case domain.IsolationStrict:
		return inv, fmt.Errorf("%w: strict isolation is not verified for claude on this machine", runner.ErrUnsupported)
	case domain.IsolationSoft:
		// Sealed answers must not land in plaintext transcripts under
		// ~/.claude/projects, so the session is not persisted. This also
		// means the run cannot be resumed.
		if req.NativeSessionID != "" {
			return inv, fmt.Errorf("%w: resume is not available with soft isolation", runner.ErrUnsupported)
		}
		args = append(args, "--no-session-persistence")
	case domain.IsolationNone, "":
		if req.NativeSessionID != "" {
			args = append(args, "--resume", req.NativeSessionID)
		} else {
			args = append(args, "--session-id", req.RunID.String())
		}
	default:
		return inv, fmt.Errorf("%w: isolation %q", runner.ErrUnsupported, req.Isolation)
	}

	settings := map[string]any{}
	switch req.Tools {
	case domain.ToolsNone, "":
		args = append(args, "--tools", "")
	case domain.ToolsReadOnly:
		args = append(args, "--tools", "Read,Glob,Grep")
		settings["permissions"] = map[string]any{"blockReadsOutsideWorkingDirectories": true}
	case domain.ToolsWorkspaceWrite:
		args = append(args, "--tools", "Read,Glob,Grep,Edit,Write,Bash", "--permission-mode", "acceptEdits")
		settings["permissions"] = map[string]any{"blockReadsOutsideWorkingDirectories": true}
	default:
		return inv, fmt.Errorf("%w: tool policy %q", runner.ErrUnsupported, req.Tools)
	}
	if len(settings) > 0 {
		raw, err := json.Marshal(settings)
		if err != nil {
			return inv, err
		}
		args = append(args, "--settings", string(raw))
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	if req.Instructions != "" {
		if len(req.Instructions) > MaxArgBytes {
			return inv, fmt.Errorf("%w: instructions over %d bytes", runner.ErrUnsupported, MaxArgBytes)
		}
		args = append(args, "--append-system-prompt", req.Instructions)
	}

	inv.Args = args
	inv.Stdin = req.Prompt
	return inv, nil
}
