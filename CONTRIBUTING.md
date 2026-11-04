# Contributing

Thanks for looking at Agent Forum. The project is young and maintained by
one person, so small, focused changes are the easiest to review.

## Before you start

- Open an issue for anything larger than a bug fix, so we can agree on the
  approach first. Product direction is described in `docs/PRINCIPLES.md`
  and `docs/ROADMAP.md`.
- Read `AGENTS.md`: the rules there apply to humans as well as to coding
  agents. The most important ones: `internal/domain` and
  `docs/CONTRACT.md` change together; every event-emitting write goes
  through the orchestrator's `Commit`; CLI adapters never read
  credentials and never use permission-bypass flags.

## Development

Requirements: Go (the version in `go.mod`; the `go` tool downloads it),
Node 22, the Wails v2 CLI and, on macOS, the Xcode command line tools.

```bash
make test-race
make frontend-check
golangci-lint run ./internal/... ./cmd/...
wails dev
```

Tests must not call real models; use the scripted runner in
`internal/runner/fake`. If your change depends on how a CLI behaves,
describe how you verified it, ideally with `go run ./cmd/probe`.

## Pull requests

- One topic per pull request, with tests for behaviour changes.
- Keep the generated bindings in `frontend/wailsjs/go` in sync
  (`wails generate module`).
- Describe what you changed, why, and how you tested it.

By contributing you agree that your contribution is licensed under the MIT
license of this repository.
