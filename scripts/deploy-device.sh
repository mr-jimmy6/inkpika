#!/bin/sh
# Internal SSH installer. Only the host-side deploy.sh should invoke this script.
# All device commands must work with POSIX sh and BusyBox.
set -eu
umask 077
STAGE=$1
APPDIR=/mnt/us/extensions/inkpika
CHANGED=0

fail() { printf '%s\n' "$1" >&2; exit 1; }

rollback() {
    while IFS= read -r name; do
        cp -p "$STAGE/backup/$name" "$STAGE/restore" || return 1
        mv -f "$STAGE/restore" "$APPDIR/$name" || return 1
    done < "$STAGE/existing-files"
    while IFS= read -r name; do
        rm -f "$APPDIR/$name" || return 1
    done < "$STAGE/added-files"
}

finish() {
    result=$?
    trap - 0 HUP INT TERM
    if [ "$result" -ne 0 ] && [ "$CHANGED" -eq 1 ]; then
        if rollback; then
            printf '%s\n' 'Installation failed; previous program files restored. Services remain stopped.' >&2
        else
            printf '%s\n' 'Rollback incomplete; retain the recovery directory and restore files before starting services.' >&2
        fi
    fi
    exit "$result"
}
trap finish 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# Validate bytes and destination types before stopping anything.
(cd "$STAGE/payload" && sha256sum -c "$STAGE/SHA256SUMS")
for directory in "$APPDIR" "$APPDIR/bin" "$APPDIR/ui"; do
    [ ! -L "$directory" ] || fail 'Refusing to install through a symlink.'
    if [ -e "$directory" ] && [ ! -d "$directory" ]; then
        fail 'An installation directory is not a directory.'
    fi
done
while read -r digest name; do
    [ ! -L "$APPDIR/$name" ] || fail 'Refusing to replace a symlink.'
    if [ -e "$APPDIR/$name" ] && [ ! -f "$APPDIR/$name" ]; then
        fail 'An installation file is not a regular file.'
    fi
done < "$STAGE/SHA256SUMS"

if [ -f "$APPDIR/bin/inkpika" ]; then
    for script in settings-stop.sh transfer-stop.sh; do
        [ -f "$APPDIR/$script" ] || fail 'Existing installation is missing a stop script.'
        sh "$APPDIR/$script"
    done
    if [ -f "$APPDIR/config.json" ]; then
        sh "$APPDIR/stop.sh"
    fi
fi
# Also catch a scan-once command or a process whose config/PID file is missing.
for process in /proc/[0-9]*/exe; do
    executable=$(readlink "$process" 2>/dev/null || true)
    case "$executable" in
        "$APPDIR/bin/inkpika"|"$APPDIR/bin/inkpika (deleted)")
            fail 'A InkPika process is still running; retry after it stops.' ;;
    esac
done

mkdir -p "$STAGE/backup"
: > "$STAGE/existing-files"
: > "$STAGE/added-files"
while read -r digest name; do
    if [ -f "$APPDIR/$name" ]; then
        mkdir -p "$STAGE/backup/$(dirname "$name")"
        cp -p "$APPDIR/$name" "$STAGE/backup/$name"
        printf '%s\n' "$name" >> "$STAGE/existing-files"
    else
        printf '%s\n' "$name" >> "$STAGE/added-files"
    fi
done < "$STAGE/SHA256SUMS"
# Keep recovery files even if the connection disappears during replacement.
touch "$STAGE/keep"
CHANGED=1
for directory in "$APPDIR" "$APPDIR/bin" "$APPDIR/ui"; do
    if [ ! -d "$directory" ]; then mkdir -m 755 "$directory"; fi
done
while read -r digest name; do
    case "$name" in
        bin/inkpika|*.sh) chmod 755 "$STAGE/payload/$name" ;;
        *) chmod 644 "$STAGE/payload/$name" ;;
    esac
    # Same filesystem rename avoids truncating a running or previous executable.
    mv -f "$STAGE/payload/$name" "$APPDIR/$name"
done < "$STAGE/SHA256SUMS"
(cd "$APPDIR" && sha256sum -c "$STAGE/SHA256SUMS")
sync
CHANGED=0
rm -rf "$STAGE/payload"
printf '%s\n' 'Installed files verified. Configurations, book files and persistent records were not replaced.'
