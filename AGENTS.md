# Instructions for coding agents

This file is read by Codex and Claude Code before they work in this
repository. Humans: the same rules apply, see also CONTRIBUTING.md.

## What this is

A local desktop forum where a human and AI coding agents (Claude Code and
Codex CLIs, driven as child processes) discuss, decide and keep a shared
context. Wails v2 shell, Go core under `internal/`, React + TypeScript UI
under `frontend/`, SQLite through the pure Go driver.

Read before changing anything substantial:

- `docs/CONTRACT.md`: entities, states, events, operations, flag mapping.
- `docs/adr/`: decisions and their reasons.
- `docs/PRINCIPLES.md`: product principles and what is not yet decided.
- `docs/ROADMAP.md`: what is planned, in which order.
- `docs/EXPERIMENTS.md`: what must be verified on the author's machine.

## Commands

```bash
make test-race        # Go core with the race detector; no display needed
make vet              # go vet for internal/ and cmd/
golangci-lint run ./internal/... ./cmd/...
make frontend-check   # npm ci, typecheck, unit tests, build
wails dev             # desktop app with live reload (macOS)
wails generate module # regenerate frontend/wailsjs after changing bindings
go run ./cmd/probe -provider claude -print-args "Reply with OK"   # argv only
go run ./cmd/probe -provider claude "Reply with OK"               # real CLI call
go run ./cmd/forumctl help                                        # backup, export
```

All of these must pass before a change is merged. CI runs them on pushes to
`main`, `claude/**` and `codex/**` and on pull requests.

## Rules

1. **One owner for the contract.** `internal/domain` and
   `docs/CONTRACT.md` change together, deliberately: add fields, never
   rename them; add states only with their transitions and tests. Adapters
   implement the contract; they do not change it.
2. **One write path.** Every write that emits events goes through
   `orchestrator.Service.Commit`, so events are published in sequence
   order. Do not append events with `storage.Tx.AppendEvent` from
   elsewhere.
3. **Adapters are pure.** `BuildInvocation` maps a request to argv, stdin
   and environment and is unit-tested. Every flag must be backed by the
   CLI's documentation or by a recorded result in `docs/EXPERIMENTS.md`.
   Never add permission-bypass flags (`--dangerously-skip-permissions`,
   `--dangerously-bypass-approvals-and-sandbox`, `bypassPermissions`).
   Never read, copy or store CLI credentials.
4. **Isolation is a measured property.** An adapter reports
   `MaxIsolation: strict` only after the canary experiment in
   `docs/EXPERIMENTS.md` passed on the machine and the result is recorded
   there. Prompts are not isolation.
5. **No paid calls in tests.** Tests use `internal/runner/fake`; CI never
   calls a model. Real CLI checks go through `cmd/probe` on the author's
   machine.
6. **Bindings stay in sync.** After changing an exported method of `App`
   (`app.go`) or a struct it returns, run `wails generate module` and
   commit `frontend/wailsjs/go`. CI fails on drift.
7. **Principles are not scope.** `docs/PRINCIPLES.md` lists accepted
   principles and candidate features. Do not implement a candidate (result
   states, disagreement tracking, per-participant visibility, round
   budgets, worktrees) without an ADR or a roadmap entry agreed with the
   author.
8. **Data safety.** Migrations are append-only files in
   `internal/storage/migrations`; never edit an applied migration. The
   database is backed up automatically before pending migrations run.
   Exports never include sealed messages, local paths or native session
   IDs.

## Workflow

- One branch per task: `codex/<task>` or `claude/<task>`. Small commits
  with a body that says why. Merge only with green CI.
- Do not commit build output (`build/bin`, `frontend/dist`, the root
  `agent-forum` binary), databases, `.env` files or credentials.
- Code, comments and docs are in English. Discussion with the author may
  be in Russian.
- When an experiment on the author's machine contradicts an assumption,
  fix the adapter and its tests and record the result in
  `docs/EXPERIMENTS.md`; do not adjust the experiment to fit the code.
