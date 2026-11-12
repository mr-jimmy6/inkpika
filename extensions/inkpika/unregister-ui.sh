#!/bin/sh
# Run before removing this extension. Keeps downloaded books and configuration.
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
sh "$APPDIR/settings-stop.sh"
sh "$APPDIR/transfer-stop.sh"
if [ -f "$APPDIR/config.json" ]; then sh "$APPDIR/stop.sh"; fi
DB=/var/local/appreg.db
EXPECTED='/usr/bin/mesquite -l com.inkpika.app -c file:///mnt/us/extensions/inkpika/ui/ -j'
EXISTING=$(sqlite3 "$DB" "SELECT value FROM properties WHERE handlerId='com.inkpika.app' AND name='command';")
if [ -n "$EXISTING" ] && [ "$EXISTING" != "$EXPECTED" ]; then
    printf '%s\n' 'InkPika: application ID belongs to another command.' >&2
    exit 1
fi
sqlite3 -cmd '.timeout 3000' "$DB" "BEGIN;
DELETE FROM associations WHERE handlerId='com.inkpika.app';
DELETE FROM properties WHERE handlerId='com.inkpika.app';
DELETE FROM handlerIds WHERE handlerId='com.inkpika.app';
COMMIT;"
rm -f "$APPDIR/ui/launch.js"
rm -f "$APPDIR/ui/service"
printf '%s\n' 'InkPika: launcher unregistered.'
