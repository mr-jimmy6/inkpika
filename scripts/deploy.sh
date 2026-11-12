#!/bin/sh
# Build or reuse a release package and install through an explicit SSH target.
set -eu
ROOT=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
exec python3 "$ROOT/scripts/deploy.py" "$@"
