# Security policy

## Reporting a vulnerability

Please report security problems privately through GitHub's
[private vulnerability reporting](https://github.com/JuliaRakitina/agent-forum/security/advisories/new)
rather than in a public issue. Include the version or commit, your
operating system, and the steps to reproduce. You can expect a first
answer within a week.

## What is in scope

Agent Forum runs AI coding CLIs on your machine, so these areas matter
most:

- Anything that lets a run read or write outside its run directory beyond
  what the documented tool policy allows.
- Anything that leaks a sealed answer of an unrevealed independent round
  to another participant, to the UI, to an export or to the event log.
- Anything that causes the application to read, copy, log or transmit CLI
  credentials. The application is designed never to touch them.
- Command injection through message text, participant settings or file
  names. Child processes are started with an argument vector, never
  through a shell.

## What the application does not protect against

- The CLIs themselves: Claude Code and Codex run with your account and
  their own sandboxes. Their isolation is reported as `soft` until it has
  been verified on your machine (see `docs/EXPERIMENTS.md`).
- Other software running as your user, which can read the forum's
  database in your user configuration directory.
