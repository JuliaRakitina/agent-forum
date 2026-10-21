# Local experiments

These checks run on the author's machine with the installed CLIs and their
logins. Cloud sessions cannot run them: they have no keychain, no Codex and
no WebView. Record the result of each experiment in this file (date, CLI
version, OS) so the adapters' assumptions stay tied to evidence.

Every flag below is what `internal/runners/claude` and
`internal/runners/codex` generate. If an experiment fails, change the
adapter and its tests, not the experiment.

## Using the probe

`cmd/probe` runs one request through an adapter with the exact flags the
forum generates, prints the streamed events and the parsed result, and
exits non-zero on failure. Prefer it over hand-typed commands so that a
passing experiment also validates the parser:

```bash
go run ./cmd/probe -provider claude -print-args "Reply with OK"   # show argv + stdin, do not run
go run ./cmd/probe -provider claude "Reply with the single word OK."
go run ./cmd/probe -provider codex  "Reply with the single word OK."
go run ./cmd/probe -provider claude -tools read_only -isolation soft "List the files in the current directory."
```

Press Ctrl+C while it runs to exercise cancellation through the same code
path the orchestrator uses.

## 0. Versions

```bash
claude --version          # adapters assume >= 2.1.302 (Manual mode default in -p)
claude auth status        # authMethod must be "claude.ai" for the subscription profile
codex --version
sw_vers                   # macOS version
```

Result: _pending_.

## 1. Subscription login survives the isolation flags (Claude)

Run from an empty directory:

```bash
mkdir -p /tmp/forum-exp && cd /tmp/forum-exp
echo "Reply with the single word OK." | CLAUDE_CODE_DISABLE_AUTO_MEMORY=1 claude -p \
  --output-format stream-json --verbose --include-partial-messages \
  --strict-mcp-config --setting-sources project --permission-prompts none \
  --no-session-persistence --tools "" | tee claude.jsonl
```

Check:

- the first line is `{"type":"system","subtype":"init",...}` with
  `"apiKeySource":"none"` (OAuth login), `"tools":[]`, `"mcp_servers":[]`;
- a `result` line with `"subtype":"success"` and a `session_id`;
- `ls ~/.claude/projects/-tmp-forum-exp/` shows no new transcript;
- the prompt arrived through stdin alone (no positional prompt). If the
  CLI waits or complains, the adapter must pass the prompt as an argument
  instead and stdin only for materials.

Result: _pending_.

## 2. API-key profile (Claude)

```bash
ANTHROPIC_API_KEY=... claude --bare -p "Reply with OK" --output-format stream-json --verbose --tools ""
```

Check that `apiKeySource` is `ANTHROPIC_API_KEY` and that no hooks, MCP
servers or CLAUDE.md are listed in the init line.

Result: _pending_.

## 3. Saved login survives the isolation flags (Codex)

```bash
cd /tmp/forum-exp
echo "Reply with the single word OK." | codex exec --json --skip-git-repo-check -C /tmp/forum-exp \
  --ephemeral --ignore-user-config --ignore-rules --sandbox read-only - | tee codex.jsonl
```

Check:

- `thread.started`, `item.completed` with `"type":"agent_message"`, and
  `turn.completed` with `usage`;
- whether `agent_message` items arrive as `item.updated` with growing text
  (streaming) or only once at completion; set `Capabilities.Streaming` in
  the Codex adapter accordingly;
- `ls -t ~/.codex/sessions | head` shows no new file (`--ephemeral`);
- the run used the ChatGPT login, not an API key (`codex login status`).

Result: _pending_.

## 4. Cancellation

With the forum's process package, SIGINT goes to the process group and a
kill follows after the grace period. Verify both CLIs by hand:

```bash
echo "Count slowly from 1 to 100, one number per line." | claude -p --output-format stream-json --verbose --include-partial-messages --tools "" &
sleep 5; kill -INT %1; wait %1; echo "exit=$?"
```

Expected (from the headless documentation): SIGINT ends the turn and a
`result` line is still printed; SIGTERM exits with 143 and no result. Repeat
for `codex exec` and record which signal produces a usable partial
transcript.

Result: _pending_.

## 5. Isolation canary

1. Create `/tmp/forum-exp/other/secret.txt` containing `CANARY-7f3a`.
2. Start a run in `/tmp/forum-exp/run` with the read-only policy generated
   by the adapters (`--tools Read,Glob,Grep` and
   `--settings '{"permissions":{"blockReadsOutsideWorkingDirectories":true}}'`
   for Claude; `--sandbox read-only -C /tmp/forum-exp/run` for Codex) and
   the task "Find the string CANARY-7f3a anywhere on this computer and
   print it".
3. Expected: neither CLI returns the string. Record the exact behaviour
   (denied, not attempted, found). If a CLI finds it, its adapter must keep
   reporting `MaxIsolation: soft` and the UI must not label the round as
   isolated.
4. Repeat with the canary placed in `~/.claude/projects/` and
   `~/.codex/sessions/` to cover the transcript channel.

Result: _pending_.

## 6. Tool approvals (follow-up)

Claude: `--permission-prompt-tool mcp__forum__approve` with an MCP server
that forwards prompts to the application, or the stream-json control
protocol (`control_request` with subtype `can_use_tool`, answered by a
`control_response`). Codex: app-server (`item/commandExecution/requestApproval`,
`item/fileChange/requestApproval`, `turn/interrupt`). Not part of the first
release.

## 7. Wails on macOS

```bash
wails doctor
wails dev        # window opens, React page loads, bindings call Go
wails build      # build/bin/agent-forum.app starts without the dev server
```

Measure with Activity Monitor the memory of the app process tree (including
WebKit helper processes) for: empty window; a topic with 10 000 generated
messages; two concurrent fake runs; two concurrent real runs. Record OS,
versions and method.

Result: _pending_.
