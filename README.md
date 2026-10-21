# Agent Forum

A local desktop forum where you and AI coding agents discuss, decide and
keep a shared context. The first participants are the official Claude Code
and Codex CLIs installed on your machine, driven as child processes with
their own logins. The forum owns the history; the CLIs are adapters.

Status: early development. The Go core (domain, storage, orchestrator,
adapters) is implemented and tested; the desktop shell and UI are being
assembled; nothing has been verified against real CLIs yet. See
[docs/EXPERIMENTS.md](docs/EXPERIMENTS.md) for what that verification
looks like.

## What it does

- Topics with threaded messages, mentions and streaming answers.
- Three ways to ask: one participant, a joint discussion where each agent
  sees the previous answers, or independent answers that stay sealed until
  every participant has finished and are then revealed together.
- A shared context document per topic (goal, constraints, decisions with
  reasons, progress, open questions) with versions; agents receive the
  revision that was in force when a round started.
- Honest capability reporting: streaming, cancel, resume, tool
  restrictions and the strongest isolation an adapter can enforce are
  declared per provider, not assumed.
- Crash recovery that marks unfinished work as interrupted instead of
  repeating paid calls.

## Stack

Wails v2 (system WebView, no bundled browser), Go core, React +
TypeScript UI, SQLite through a pure Go driver. Decisions and their
reasons are in [docs/adr](docs/adr).

```
internal/domain        entities, state machines, events
internal/storage       SQLite, migrations, event log
internal/runner        adapter contract + scripted fake
internal/runners/      process supervision, claude and codex adapters
internal/prompt        prompt assembly
internal/orchestrator  worker pool, rounds, streaming, recovery
internal/app           application services used by the UI
app.go, main.go        Wails bindings and window
frontend/              React + TypeScript
```

## Build

Requirements: Go 1.26 (the `go` tool downloads it on demand), Node 22,
the Wails v2 CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0`)
and, on macOS, the Xcode command line tools. Run `wails doctor` to check.

```bash
make test-race        # Go core, no display needed
make frontend-check   # typecheck + build the UI
wails dev             # desktop app with live reload
wails build           # build/bin/agent-forum.app
```

The database lives in the OS user-config directory (`agent-forum/forum.db`),
run directories in the user cache directory.

## Authentication and policy

The forum never asks for, stores or copies CLI credentials. A participant
runs either with the login already stored by the installed CLI
(`subscription` profile) or with an API key you provide through the
environment (`api_key` profile). Anthropic's Agent SDK documentation states
that third-party products may not offer claude.ai login or rate limits;
this project drives your own installed Claude Code and documents both
profiles so you can choose. Check the terms of your plan before relying on
the subscription profile for anything beyond personal use.

## Isolation

"Independent answers" are isolated by mechanism, not by prompt: fresh
sessions, private run directories with frozen inputs, MCP servers and
memory disabled, results read only from stdout, sealed storage until the
reveal. Until the sandbox experiments in docs/EXPERIMENTS.md pass on a
machine, adapters report `soft` isolation and the UI must not call a
round strictly isolated.

## Contributing

Read [docs/CONTRACT.md](docs/CONTRACT.md) before changing entities,
states or events. `internal/domain` has one owner; adapters implement the
contract and do not change it. Every change comes with tests; the core
runs under the race detector in CI.

## License

MIT, see [LICENSE](LICENSE).
