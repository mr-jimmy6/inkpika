#!/bin/sh
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
BIN="$APPDIR/bin/inkpika"
# Close our UI before invalidating its session. No effect on the system browser.
UI_SERVICE=$(cat "$APPDIR/ui/service" 2>/dev/null || true)
if [ "$UI_SERVICE" != transfer ] && [ "$UI_SERVICE" != settings ]; then
    lipc-set-prop com.lab126.appmgrd stop app://com.inkpika.app 2>/dev/null || true
    rm -f "$APPDIR/ui/launch.js" "$APPDIR/ui/service"
fi
STATE=$("$BIN" -config "$APPDIR/config.json" -state-dir 2>"$APPDIR/startup.log")
trap 'sh "$APPDIR/menu.sh" refresh' EXIT
if [ ! -f "$STATE/server.pid" ]; then exit 0; fi
PID=$(cat "$STATE/server.pid")
case "$PID" in ''|*[!0-9]*) exit 1;; esac
if [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ]; then
    kill -TERM "$PID" 2>/dev/null || true
    ATTEMPT=0
    while [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ] && [ "$ATTEMPT" -lt 6 ]; do
        sleep 1
        ATTEMPT=$((ATTEMPT + 1))
    done
    if [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ]; then
        printf '%s\n' 'InkPika: still stopping; try again shortly.' >&2
        exit 1
    fi
    printf '%s\n' 'InkPika: stopped.' >&2
fi
