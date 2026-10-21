# Contract: entities, states, events, operations

This is the shared contract between the Go core, the CLI adapters and the
UI. Change it deliberately: add fields, never rename them; add states only
with their transitions; keep `internal/domain` as the single owner.

## Entities

| Entity | Purpose | Key fields |
| --- | --- | --- |
| Workspace | A project folder with its own topics and participants | `id`, `name`, `rootDir` |
| Topic | A thread; questions and tasks are kinds, not a separate tracker | `kind`: `discussion` \| `question` \| `task` |
| Participant | A configured agent (or the human) | `provider`, `model`, `role`, `effort`, `maxWords`, `mode`, `profile`, `tools`, `enabled` |
| Message | One immutable post; corrections are new messages with `correctsId` | `authorKind`, `participantId`, `body`, `mentions`, `runId`, `roundId`, `visibility` |
| Run | One CLI invocation for one participant | `status`, `profile`, `tools`, `isolation`, `workDir`, `nativeSessionId`, `usage`, `error` |
| Round | A coordinated set of runs | `kind`, `status`, `prompt`, `participantIds`, `inputs` (frozen), `autoReveal` |
| ContextRevision | A version of the shared context document | `version`, `status`, `doc` |
| ArtifactVersion | A versioned file attached to a topic (schema only in this release) | `name`, `version`, `sha256`, `path` |
| Event | Append-only log entry delivered to the UI | `seq`, `type`, `topicId`, `payload` |

## Enumerations

- `Provider`: `human`, `claude`, `codex`, `fake`.
- `ParticipationMode`: `mentioned` (default), `always`, `manual`.
- `RunProfile`: `subscription`, `api_key`. See ADR 0003.
- `ToolPolicy`: `none`, `read_only`, `workspace_write`.
- `IsolationLevel`: `none`, `soft`, `strict`. Adapters report the strongest
  level they can enforce in `Capabilities.maxIsolation`; `strict` is only
  reported after a verified experiment on the machine.
- `Visibility`: `visible`, `sealed`.
- `RoundKind`: `single`, `joint`, `independent`.
- `RevisionStatus`: `accepted`, `proposed`, `discarded`.

## State machines

Run:

```
queued ──▶ running ──▶ succeeded | failed | cancelled | timed_out
   │                      
   └──▶ cancelled      queued/running ──▶ interrupted (on recovery only)
```

Round:

```
running ──▶ awaiting_reveal ──▶ revealed
running ──▶ revealed                      (single/joint, or auto-reveal)
running ──▶ partial ──▶ running (retry) | revealed (explicit) | cancelled
running | awaiting_reveal ──▶ cancelled
running ──▶ interrupted (on recovery only)
```

Rules enforced by the orchestrator:

- Independent rounds store results as sealed messages. Sealed bodies never
  leave storage until the reveal, which happens in one transaction.
- A failed, timed-out or cancelled run never reveals anything; the round
  becomes `partial` and waits for the human.
- Joint rounds run participants in order; each later run sees the earlier
  answers. A failure stops the sequence.
- Recovery marks unfinished runs and rounds `interrupted`; nothing is
  restarted automatically.

## Events

Every event has `seq` (assigned by storage, strictly increasing), `type`,
`topicId` (empty for workspace-level events), `at` and `payload`. The UI
keeps the last `seq` it applied; after a reconnect or a `forum:resync`
signal it calls `Events(after)` and applies the missing events in order.

| Type | Payload | Notes |
| --- | --- | --- |
| `topic.created` | `{topic}` | |
| `message.created` | `{message}` | For sealed messages `body` is empty |
| `message.revealed` | `{message}` | Full body; one per revealed message |
| `run.status_changed` | `{runId, participantId, roundId, status, error, usage}` | `usage` present on terminal states |
| `run.output` | `{runId, index, text, sealed}` | Coalesced chunks; `text` is empty when `sealed` |
| `run.tool_activity` | `{runId, tool, summary, status}` | Summary only, never raw tool input or output |
| `round.status_changed` | `{roundId, status}` | |
| `context.revised` | `{revision}` | New revision or status change |
| `participant.changed` | `{participant}` | |

## Operations (Wails bindings, `app.go`)

| Method | Effect |
| --- | --- |
| `EnsureWorkspace()` | First workspace, created with default participants when missing |
| `ListWorkspaces()`, `CreateWorkspace(name, rootDir)` | |
| `ListParticipants(workspaceId)`, `SaveParticipant(participant)` | Validates effort and profile against the adapter's capabilities |
| `ListTopics(workspaceId)`, `CreateTopic(workspaceId, title, kind)` | |
| `GetThread(topicId, limit)` | Topic, newest `limit` visible messages, runs, rounds, participants, current context, `lastEventSeq` |
| `PostMessage({topicId, body, parentId, mentions, mode, autoReveal})` | Stores the message; starts a round for mentioned and "always" participants: `one` → single (joint when several), `joint`, `independent` |
| `CancelRun(runId)`, `CancelRound(roundId)` | |
| `RevealRound(roundId)` | From `awaiting_reveal` or, explicitly, `partial` |
| `RetryRound(roundId)` | New runs for participants without a successful run |
| `RunOutput(runId)` | Streamed text; error while the round is sealed |
| `SaveContext(topicId, doc)` | New accepted revision authored by the human |
| `ListContextRevisions(topicId)`, `SetContextRevisionStatus(id, status)` | Accept or discard a proposal |
| `Events(after, limit)` | Event log replay |
| `Capabilities()` | One entry per provider |

## Runner contract (`internal/runner`)

```go
type Runner interface {
    Capabilities() Capabilities
    Run(ctx context.Context, req Request, sink Sink) (Result, error)
}
```

- `Request`: run ID, instructions, prompt, model, effort, private work
  directory, profile, tool policy, isolation level, timeout, optional
  native session to resume, extra environment.
- Events: `started` (native session ID), `text_delta`, `tool` (name,
  status, short summary), `status`, `log`.
- Errors: `ErrCancelled`, `ErrTimeout`, `ErrUnsupported`, `ErrProcess`.
  Cancellation returns the partial result together with the error.
- Adapters are pure from request to argv + stdin + env (`BuildInvocation`),
  which is what the unit tests cover. Process handling lives in
  `internal/runners/process`: argv only, stdin delivery, bounded stderr,
  SIGINT to the process group, SIGKILL after a grace period.

### Flag mapping

| Policy | Claude Code | Codex |
| --- | --- | --- |
| always | `-p --output-format stream-json --verbose --include-partial-messages --permission-prompts none` | `exec --json --skip-git-repo-check -C <dir> -` (prompt on stdin) |
| profile `subscription` | `--strict-mcp-config --setting-sources project`, env `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` | stored login in `CODEX_HOME` |
| profile `api_key` | `--bare` (needs `ANTHROPIC_API_KEY`) | needs `CODEX_API_KEY` in the run environment |
| isolation `none` | `--session-id <runId>` or `--resume <native>` | optional `resume <thread>` |
| isolation `soft` | `--no-session-persistence` | `--ephemeral --ignore-user-config --ignore-rules` |
| isolation `strict` | rejected until verified | rejected until verified |
| tools `none` | `--tools ""` | `--sandbox read-only` (closest available) |
| tools `read_only` | `--tools Read,Glob,Grep` + `permissions.blockReadsOutsideWorkingDirectories` | `--sandbox read-only` |
| tools `workspace_write` | `--tools Read,Glob,Grep,Edit,Write,Bash --permission-mode acceptEdits` + read block | `--sandbox workspace-write` |
| model / effort | `--model`, `--effort` | `-m`, `-c model_reasoning_effort="…"` |
| instructions | `--append-system-prompt` | prepended to the prompt |

Sources: Claude Code CLI reference and headless guide (code.claude.com,
2026-10-20); Codex `codex-rs/exec/src/cli.rs` and `exec_events.rs`
(github.com/openai/codex, 2026-10-20). docs/EXPERIMENTS.md lists what still
has to be confirmed on the author's machine.
