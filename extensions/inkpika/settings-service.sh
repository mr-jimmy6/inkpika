#!/bin/sh
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
BIN="$APPDIR/bin/inkpika"
umask 077
STATE=$("$BIN" -settings-state-dir -config "$APPDIR/config.json" 2>"$APPDIR/settings-startup.log")
running() {
    [ -f "$STATE/server.pid" ] || return 1
    PID=$(cat "$STATE/server.pid")
    case "$PID" in ''|*[!0-9]*) return 1;; esac
    [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ] && kill -0 "$PID" 2>/dev/null &&
        tr '\000' '\n' <"/proc/$PID/cmdline" | grep -qx -- '-settings'
}
if [ "${1:-}" = status ]; then running; exit $?; fi
mkdir -p "$STATE"
if ! running; then
    rm -f "$STATE/launch.url" "$STATE/server.pid"
    if [ -f "$STATE/server.log" ]; then mv -f "$STATE/server.log" "$STATE/server.previous.log"; fi
    GOMAXPROCS=1 GOGC=50 nohup "$BIN" -settings -config "$APPDIR/config.json" -kual-menu "$APPDIR/menu.json" </dev/null >"$STATE/server.log" 2>&1 &
    ATTEMPT=0
    while [ "$ATTEMPT" -lt 15 ]; do
        if [ -s "$STATE/launch.url" ] && running; then break; fi
        sleep 1
        ATTEMPT=$((ATTEMPT + 1))
    done
fi
if ! running || [ ! -s "$STATE/launch.url" ]; then
    command -v eips >/dev/null 2>&1 && eips 1 3 'Configuration service: see settings-state/server.log'
    exit 1
fi
sh "$APPDIR/menu.sh" refresh
