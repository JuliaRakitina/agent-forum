# ADR 0003: CLI adapters, run profiles and isolation levels

Date: 2026-10-21. Status: accepted.

## Decision

Agents are driven through the official CLIs as child processes with
structured output (`claude -p --output-format stream-json`,
`codex exec --json`). The application never reads or stores CLI
credentials and never prompts for a claude.ai or ChatGPT login itself.

Each run has a **profile** and an **isolation level**:

- `subscription`: the CLI's stored login is used. For Claude Code this
  rules out `--bare` (bare mode ignores the stored login), so the adapter
  disables hooks, MCP servers, project settings and auto memory with
  individual flags.
- `api_key`: the CLI starts in its minimal mode (`--bare` for Claude Code)
  with the key taken from the run environment.
- `none`: the run behaves like the CLI in a terminal.
- `soft`: fresh session, private run directory with frozen inputs, no MCP
  servers, no memory, no session files on disk, results read only from
  stdout. Reads outside the run directory are discouraged, not proven
  impossible.
- `strict`: `soft` plus an OS or CLI sandbox verified on this machine.
  Adapters report `strict` only after that verification; until then they
  reject requests for it.

Tool approvals are not offered by the first adapters. Claude Code supports
them through an MCP permission-prompt tool or the stream-json control
protocol; Codex only through its app-server transport. Both are tracked as
follow-up work.

## Why

- The concept requires honesty about isolation: a prompt that says "do not
  read other answers" is not isolation, and neither is a separate git
  worktree. Flags are the mechanism; the UI label follows the flags.
- Anthropic's Agent SDK documentation states that third-party products may
  not offer claude.ai login or rate limits. The forum therefore drives the
  user's own installed CLI and offers an API-key profile as the clean
  alternative. README states this.

## Consequences

- Adapters are pure functions from a request to argv + stdin + env, which
  makes the flag mapping unit-testable without a CLI.
- Every flag the adapters rely on is listed in docs/EXPERIMENTS.md with the
  command to verify it on the target machine.
