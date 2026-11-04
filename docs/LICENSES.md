# Dependency licences

Checked on 2026-11-04 with `scripts/licenses.sh` (go-licenses v1.6.0 for
Go, the `license` field of each package for npm). Every dependency that
ships in the application is under a permissive licence compatible with the
project's MIT licence; nothing is copyleft.

## Go modules linked into the app and the commands

The graph was taken for windows/amd64 without cgo, the only target the
tool can load from Linux; the macOS build uses the same modules apart from
Wails' platform packages, which are part of `github.com/wailsapp/wails/v2`
(MIT).

| Module | Licence |
| --- | --- |
| github.com/dustin/go-humanize | MIT |
| github.com/google/uuid | BSD-3-Clause |
| github.com/leaanthony/go-ansi-parser | MIT |
| github.com/leaanthony/slicer | MIT |
| github.com/leaanthony/u | MIT |
| github.com/mattn/go-isatty | MIT |
| github.com/ncruces/go-strftime | MIT |
| github.com/pkg/errors | BSD-2-Clause |
| github.com/remyoudompheng/bigfft | BSD-3-Clause |
| github.com/rivo/uniseg | MIT |
| github.com/wailsapp/go-webview2 | MIT, ISC (webviewloader) |
| github.com/wailsapp/wails/v2 | MIT |
| golang.org/x/sys | BSD-3-Clause |
| modernc.org/libc | MIT |
| modernc.org/mathutil | BSD-3-Clause (the tool reports "Unknown"; the LICENSE file is the three-clause BSD text, checked by hand) |
| modernc.org/memory | BSD-3-Clause |
| modernc.org/sqlite | BSD-3-Clause |

## npm packages in the production frontend

106 packages (react, react-dom, react-markdown, remark-gfm and their
dependencies): 105 MIT, 1 ISC. Development tools (TypeScript, Vite,
Vitest) are not shipped and were not part of the check.

## Repeating the check

```bash
go install github.com/google/go-licenses@v1.6.0
(cd frontend && npm ci && npm run build)
scripts/licenses.sh
```

Re-run it when a dependency is added and update this page.
