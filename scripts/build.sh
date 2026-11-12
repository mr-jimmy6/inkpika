#!/bin/sh
# Build the PW2 release from any working directory. Linux 3.0 needs Go 1.23.
set -eu
ROOT=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
export GOTOOLCHAIN="go$(cat .go-version)"
mkdir -p dist
STAGE=$(mktemp -d "$ROOT/dist/.build.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT HUP INT TERM
PACKAGED_AT=$(python3 scripts/package.py --print-build-time)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5 go build -trimpath -buildvcs=false \
    -ldflags="-s -w -buildid= -X github.com/mr-jimmy6/inkpika/internal/buildinfo.packagedAt=$PACKAGED_AT" \
    -o "$STAGE/inkpika" ./cmd/inkpika
python3 scripts/package.py --binary "$STAGE/inkpika" --packaged-at "$PACKAGED_AT"
