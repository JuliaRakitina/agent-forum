# Common tasks. The Wails app itself is built with `wails build`; everything
# under internal/ builds and tests without a display or WebView.

GO ?= go
WAILS ?= wails

.PHONY: test test-race vet lint frontend-check build dev tidy

test:
	$(GO) test ./internal/...

test-race:
	$(GO) test -race -count=1 ./internal/...

# The root package embeds frontend/dist, which exists only after a frontend
# build, so vet covers the core and commands; `wails build` vets the rest.
vet:
	$(GO) vet ./internal/... ./cmd/...

lint: vet
	cd frontend && npm run typecheck

frontend-check:
	cd frontend && npm ci && npm run typecheck && npm test && npm run build

build:
	$(WAILS) build

dev:
	$(WAILS) dev

tidy:
	$(GO) mod tidy
