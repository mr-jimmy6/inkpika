#!/bin/sh
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
BIN="$APPDIR/bin/inkpika"
if [ "$(cat "$APPDIR/ui/service" 2>/dev/null || true)" = transfer ]; then
    lipc-set-prop com.lab126.appmgrd stop app://com.inkpika.app 2>/dev/null || true
    rm -f "$APPDIR/ui/launch.js" "$APPDIR/ui/service"
fi
STATE=$("$BIN" -transfer-state-dir -transfer-config "$APPDIR/transfer.json" 2>"$APPDIR/transfer-startup.log")
trap 'sh "$APPDIR/menu.sh" refresh' EXIT
[ -f "$STATE/server.pid" ] || exit 0
PID=$(cat "$STATE/server.pid")
case "$PID" in ''|*[!0-9]*) exit 1;; esac
if [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ] && tr '\000' '\n' <"/proc/$PID/cmdline" | grep -qx -- '-transfer'; then
    kill -TERM "$PID" 2>/dev/null || true
    ATTEMPT=0
    while [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ] && [ "$ATTEMPT" -lt 6 ]; do
        sleep 1; ATTEMPT=$((ATTEMPT + 1))
    done
    if [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ]; then
        printf '%s\n' 'Wireless transfer: still stopping; try again shortly.' >&2; exit 1
    fi
fi
