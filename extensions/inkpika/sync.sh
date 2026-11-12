#!/bin/sh
# Invoked by KUAL, with fixed command names only. Do not log the session token.
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
case "${1:-}" in start|scan|cancel|refresh) COMMAND=$1;; *) exit 1;; esac
if [ "$COMMAND" = refresh ] || [ "$COMMAND" = cancel ]; then
    if ! sh "$APPDIR/service.sh" status; then
        if [ "$COMMAND" = refresh ]; then
            "$APPDIR/bin/inkpika" -config "$APPDIR/config.json" \
                -kual-menu "$APPDIR/menu.json" -scan-once >"$APPDIR/sync-command.log" 2>&1
        else
            sh "$APPDIR/menu.sh" refresh
        fi
        exit 0
    fi
fi
. "$APPDIR/service.sh"
"$BIN" -config "$CONFIG" -sync "$COMMAND" >"$APPDIR/sync-command.log" 2>&1
