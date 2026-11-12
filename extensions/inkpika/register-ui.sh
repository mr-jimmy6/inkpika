#!/bin/sh
# Register only our own WAF application; leave the system browser unchanged.
set -eu
APPDIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
if [ "$APPDIR" != /mnt/us/extensions/inkpika ]; then
    printf '%s\n' 'Install at /mnt/us/extensions/inkpika' >&2
    exit 1
fi
DB=/var/local/appreg.db
COMMAND='/usr/bin/mesquite -l com.inkpika.app -c file:///mnt/us/extensions/inkpika/ui/ -j'
EXISTING=$(sqlite3 "$DB" "SELECT value FROM properties WHERE handlerId='com.inkpika.app' AND name='command';")
if [ -n "$EXISTING" ] && [ "$EXISTING" != "$COMMAND" ]; then
    printf '%s\n' 'InkPika: application ID already in use.' >&2
    exit 1
fi
if [ "$EXISTING" != "$COMMAND" ]; then
    sqlite3 -cmd '.timeout 3000' "$DB" "BEGIN;
INSERT INTO handlerIds VALUES ('com.inkpika.app');
INSERT INTO properties VALUES ('com.inkpika.app','command','$COMMAND');
INSERT INTO properties VALUES ('com.inkpika.app','framework','');
INSERT INTO properties VALUES ('com.inkpika.app','unloadPolicy','unloadOnPause');
COMMIT;"
fi
