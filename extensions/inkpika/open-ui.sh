#!/bin/sh
# Reuse only our WAF application for both local pages.
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
case "${2:-}" in cloud|transfer|settings) MODE=$2;; *) exit 1;; esac
URL=$(cat "$1")
if ! printf '%s\n' "$URL" | grep -Eq '^http://localhost:[0-9]+/#token=[a-f0-9]{64}$'; then
    printf '%s\n' 'InkPika: invalid local session URL.' >&2; exit 1
fi
sh "$APPDIR/register-ui.sh" >>"$APPDIR/startup.log" 2>&1
lipc-set-prop com.lab126.appmgrd stop app://com.inkpika.app 2>/dev/null || true
printf 'window.location.replace("%s");\n' "$URL" >"$APPDIR/ui/launch.js.new"
mv -f "$APPDIR/ui/launch.js.new" "$APPDIR/ui/launch.js"
printf '%s\n' "$MODE" >"$APPDIR/ui/service"
lipc-set-prop com.lab126.appmgrd start app://com.inkpika.app
