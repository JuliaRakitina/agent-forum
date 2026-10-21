# ADR 0004: Independent rounds

Date: 2026-10-21. Status: accepted.

## Decision

An independent round freezes its inputs (context revision, message IDs,
artifact IDs) when it starts. Every run receives the same frozen material
in a private run directory. Results are stored as **sealed** messages that
the UI and the context assembly never read. When every run succeeds the
round moves to `awaiting_reveal`; the reveal flips all sealed messages to
visible in one transaction and emits one event per message.

A run that fails, times out or is cancelled puts the round into `partial`.
Nothing is revealed automatically; the human retries, cancels or reveals
the partial results explicitly. A round that was running when the
application stopped is marked `interrupted` on startup and is never
restarted automatically.

## Why

- Rounds are the feature that makes the forum different from a chat with
  two bots, so their guarantees must be enforced by storage and the
  orchestrator, not by prompts.
- Automatic restarts would repeat paid calls after a crash.

## Consequences

- Sealed messages exist in the database before the reveal; exports made
  while a round is open must skip them.
- Streaming output of a sealed run is persisted for recovery but delivered
  to the UI as progress only (the `sealed` flag on `run.output`).
