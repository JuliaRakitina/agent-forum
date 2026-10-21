# Common tasks. The Wails app itself is built with `wails build`; everything
# under internal/ builds and tests without a display or WebView.

GO ?= go
WAILS ?= wails

.PHONY: test test-race vet lint frontend-check build dev tidy

test:
	$(GO) test ./internal/...

test-race:
	$(GO) test -race -count=1 ./internal/...

vet:
	$(GO) vet ./...

lint: vet
	cd frontend && npm run typecheck

frontend-check:
	cd frontend && npm ci && npm run typecheck && npm run build

build:
	$(WAILS) build

dev:
	$(WAILS) dev

tidy:
	$(GO) mod tidy
