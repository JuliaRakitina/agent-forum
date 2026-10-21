# ADR 0001: Desktop shell and core language

Date: 2026-10-21. Status: accepted.

## Decision

Wails v2 (2.17.0, the stable branch) with a Go core and a React + TypeScript
frontend generated from the official `react-ts` template. SQLite through the
pure Go driver `modernc.org/sqlite`.

## Why

- The core of the application is process supervision, a state machine for
  rounds, and a transactional store. Go fits that work and is the language
  the author wants to show.
- Wails keeps Go in the same process as the window and generates TypeScript
  bindings for Go methods. Tauri would add Rust, Electron would add Node and
  a separate Go process, Flutter would add Dart and a bridge: each is another
  runtime for one maintainer to build, sign and update.
- Wails v3 entered beta on 2 August 2026 and publishes automated nightly
  pre-releases ("the API is stable, but you may still encounter issues").
  The core does not import Wails, so moving to v3 later touches only the
  window adapter and the generated bindings.
- React was chosen over Angular because the official template exists for it
  and most Wails examples and issues use React, Vue or Svelte. The core does
  not care; the frontend can be replaced.
- A pure Go SQLite driver keeps CI and cross-compilation free of a C
  toolchain dependency on the database side. Wails itself still uses cgo for
  the WebView on macOS.

## Consequences

- `go test ./internal/...` runs anywhere; `wails build` needs the platform's
  WebView development packages (macOS: Xcode command line tools).
- Memory use is measured, not assumed: the shell has no bundled Chromium,
  but each CLI run is its own process and will dominate.
