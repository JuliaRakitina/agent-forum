# ADR 0002: The core is independent from the window toolkit

Date: 2026-10-21. Status: accepted.

## Decision

Everything under `internal/` compiles and is tested without Wails. The root
package (`main.go`, `app.go`) is a thin adapter: it binds application
service methods to the window and forwards domain events to the frontend.

Events are persisted first, with a sequence number assigned by the
database, and emitted to the UI afterwards. A UI that reconnects asks for
every event after the last sequence number it saw.

## Why

- The author needs the Go core to be reviewable and testable on its own.
- A later headless command, a different UI, or Wails v3 must not require
  changes to the orchestrator or storage.
- Persist-then-emit removes a whole class of "the UI missed an update" bugs
  and makes crash recovery a replay rather than a reconstruction.

## Consequences

- Streaming text is coalesced into chunks before it is persisted, so the
  event log stays small; the UI never receives individual tokens.
- Application services return plain structs with JSON tags so the generated
  TypeScript bindings stay readable.
