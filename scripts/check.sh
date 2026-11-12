#!/bin/sh
# Keep ignored local scratch Go programs outside the project's validation scope.
set -eu
ROOT=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
export GOTOOLCHAIN="go$(cat .go-version)"
UNFORMATTED=$(gofmt -l cmd internal)
if [ -n "$UNFORMATTED" ]; then
    printf 'Run gofmt on:\n%s\n' "$UNFORMATTED" >&2
    exit 1
fi
go test -race ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
for script in scripts/*.sh extensions/inkpika/*.sh; do sh -n "$script"; done
python3 -m unittest discover -s scripts -p 'test_*.py'
