#!/bin/sh
# Start the local service only; KUAL sync must never launch a browser.
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
BIN="$APPDIR/bin/inkpika"
CONFIG="$APPDIR/config.json"
umask 077
notify() {
    if command -v eips >/dev/null 2>&1; then
        eips 1 3 'InkPika: see startup.log'
    fi
    printf '%s\n' "$1" >&2
}
if [ ! -f "$CONFIG" ]; then notify 'InkPika: copy and edit config.example.json as config.json'; exit 1; fi
if [ ! -f "$BIN" ]; then notify 'InkPika: binary missing. Install the release ZIP.'; exit 1; fi
chmod +x "$BIN"
if ! STATE=$("$BIN" -config "$CONFIG" -state-dir 2>"$APPDIR/startup.log"); then
    notify 'InkPika: config error. See extensions/inkpika/startup.log'; exit 1
fi
running() {
    [ -f "$STATE/server.pid" ] || return 1
    PID=$(cat "$STATE/server.pid")
    case "$PID" in ''|*[!0-9]*) return 1;; esac
    [ "$(readlink "/proc/$PID/exe" 2>/dev/null || true)" = "$BIN" ] && kill -0 "$PID" 2>/dev/null
}
if [ "${1:-}" = status ]; then running; exit $?; fi
mkdir -p "$STATE"
if ! running; then
    rm -f "$STATE/launch.url" "$STATE/server.pid"
    # Rotate a single bounded log on each launch. No credentials are logged.
    if [ -f "$STATE/server.log" ]; then mv -f "$STATE/server.log" "$STATE/server.previous.log"; fi
    GOMAXPROCS=1 GOGC=50 nohup "$BIN" -config "$CONFIG" -kual-menu "$APPDIR/menu.json" </dev/null >"$STATE/server.log" 2>&1 &
    ATTEMPT=0
    while [ "$ATTEMPT" -lt 15 ]; do
        if [ -s "$STATE/launch.url" ] && running; then break; fi
        sleep 1
        ATTEMPT=$((ATTEMPT + 1))
    done
fi
if ! running || [ ! -s "$STATE/launch.url" ]; then
    notify 'InkPika: launch failed. See server.log in its state folder.'; exit 1
fi
sh "$APPDIR/menu.sh" refresh
