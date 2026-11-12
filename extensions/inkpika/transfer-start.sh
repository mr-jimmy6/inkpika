#!/bin/sh
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
. "$APPDIR/transfer-service.sh"
sh "$APPDIR/open-ui.sh" "$STATE/launch.url" transfer
