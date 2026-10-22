// Package codex adapts the Codex CLI (`codex exec --json`) to the runner
// contract. Flags follow the exec command's own definition
// (codex-rs/exec/src/cli.rs, checked 2026-10-20). Approvals are not available
// in exec mode: the headless default is never to ask, so a run either has
// the sandbox it was started with or fails. Interactive approvals need the
// app-server transport, which is a separate adapter.
package codex

import (
	"fmt"
	"strings"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/runner"
)

// Efforts lists reasoning effort values accepted by the config key
// model_reasoning_effort. The exact set depends on the model; this is the
// superset documented in the sample configuration.
var Efforts = []string{"minimal", "low", "medium", "high", "xhigh"}

// Invocation is a fully built command line.
type Invocation struct {
	Args  []string
	Env   map[string]string
	Stdin string
}

// BuildInvocation maps a request to exec flags.
//
//   - The prompt goes to stdin (`-`), never into a shell.
//   - --skip-git-repo-check: the run directory is not a repository.
//   - -C sets the working root to the run directory.
//   - --sandbox is always explicit, so the user's config.toml cannot widen
//     it. ToolsNone has no exact equivalent: exec cannot remove the shell
//     tool, so it is approximated by a read-only sandbox.
//   - Soft isolation adds --ephemeral (no session files on disk),
//     --ignore-user-config (no MCP servers or hooks from ~/.codex, auth is
//     still read from CODEX_HOME) and --ignore-rules.
//   - Instructions are prepended to the prompt because exec has no
//     system-prompt flag.
func BuildInvocation(req runner.Request) (Invocation, error) {
	inv := Invocation{Env: map[string]string{}}
	for k, v := range req.Env {
		inv.Env[k] = v
	}
	args := []string{"exec", "--json", "--skip-git-repo-check"}
	if req.WorkDir != "" {
		args = append(args, "-C", req.WorkDir)
	}

	switch req.Profile {
	case domain.ProfileSubscription, "":
		// The stored ChatGPT login in CODEX_HOME is used as is.
	case domain.ProfileAPIKey:
		if inv.Env["CODEX_API_KEY"] == "" {
			return inv, fmt.Errorf("%w: api_key profile needs CODEX_API_KEY in the run environment", runner.ErrUnsupported)
		}
	default:
		return inv, fmt.Errorf("%w: profile %q", runner.ErrUnsupported, req.Profile)
	}

	switch req.Isolation {
	case domain.IsolationStrict:
		return inv, fmt.Errorf("%w: strict isolation is not verified for codex on this machine", runner.ErrUnsupported)
	case domain.IsolationSoft:
		if req.NativeSessionID != "" {
			return inv, fmt.Errorf("%w: resume is not available with soft isolation", runner.ErrUnsupported)
		}
		args = append(args, "--ephemeral", "--ignore-user-config", "--ignore-rules")
	case domain.IsolationNone, "":
	default:
		return inv, fmt.Errorf("%w: isolation %q", runner.ErrUnsupported, req.Isolation)
	}

	switch req.Tools {
	case domain.ToolsNone, domain.ToolsReadOnly, "":
		args = append(args, "--sandbox", "read-only")
	case domain.ToolsWorkspaceWrite:
		args = append(args, "--sandbox", "workspace-write")
	default:
		return inv, fmt.Errorf("%w: tool policy %q", runner.ErrUnsupported, req.Tools)
	}

	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+quoteTOML(req.Effort))
	}

	if req.NativeSessionID != "" {
		args = append(args, "resume", req.NativeSessionID)
	}
	args = append(args, "-")

	var prompt strings.Builder
	if req.Instructions != "" {
		prompt.WriteString("## Instructions\n\n")
		prompt.WriteString(req.Instructions)
		prompt.WriteString("\n\n## Task\n\n")
	}
	prompt.WriteString(req.Prompt)

	inv.Args = args
	inv.Stdin = prompt.String()
	return inv, nil
}

// quoteTOML renders a string as a TOML basic string for -c overrides.
func quoteTOML(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
