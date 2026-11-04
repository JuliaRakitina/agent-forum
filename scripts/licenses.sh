#!/usr/bin/env bash
# Prints the licenses of the dependencies that end up in the application:
# Go modules linked into the app and the commands, and the npm packages of
# the production frontend build. See docs/LICENSES.md for the last result.
#
# Requires: go install github.com/google/go-licenses@v1.6.0
# Run from the repository root after `npm ci` in frontend/ and a frontend
# build (the root package embeds frontend/dist).
set -euo pipefail

cd "$(dirname "$0")/.."

echo "== Go modules (windows/amd64 graph, no cgo) =="
# go-licenses needs GOROOT to tell the standard library apart when the go
# command runs a downloaded toolchain.
GOROOT="$(go env GOROOT)" GOOS=windows CGO_ENABLED=0 go-licenses report . ./cmd/... 2>/dev/null |
  awk -F, '{printf "%-70s %s\n", $1, $3}'

echo
echo "== npm production dependencies =="
cd frontend
npm ls --omit=dev --all --parseable 2>/dev/null | tail -n +2 | while read -r dir; do
  node -e '
    const p = require(process.argv[1] + "/package.json");
    const l = typeof p.license === "string" ? p.license : JSON.stringify(p.license || p.licenses || "UNKNOWN");
    console.log(p.name + "@" + p.version + " " + l);
  ' "$dir"
done | sort -u
