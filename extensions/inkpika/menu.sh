#!/bin/sh
# These short-lived commands never start the HTTP service or open a window.
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
case "${1:-}" in refresh|check) COMMAND=$1;; *) exit 1;; esac
"$APPDIR/bin/inkpika" -config "$APPDIR/config.json" \
    -kual-menu "$APPDIR/menu.json" -menu-action "$COMMAND" >"$APPDIR/menu-command.log" 2>&1
