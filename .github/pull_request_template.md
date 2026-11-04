## What and why

## How it was tested

- [ ] `make test-race`
- [ ] `golangci-lint run ./internal/... ./cmd/...`
- [ ] `make frontend-check`
- [ ] Bindings regenerated (`wails generate module`) if `App` methods or their structs changed
- [ ] Real CLI behaviour checked with `cmd/probe` (only for adapter changes)

## Contract

- [ ] No change to `internal/domain` or `docs/CONTRACT.md`
- [ ] Contract changed; `docs/CONTRACT.md` updated in this pull request
