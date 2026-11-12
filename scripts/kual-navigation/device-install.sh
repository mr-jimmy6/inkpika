#!/bin/sh
# Run on the Kindle. Only the exact allowlisted KUAL build is supported.
set -eu
TARGET=/opt/amazon/ebook/booklet/KUALBooklet.jar
BACKUP=/var/local/inkpika/kual-navigation
ORIGINAL=c13fc2bbb2890530a5c6374b879b2049a6136028713c37794d9f40093ea8dae3
hash() { sha256sum "$1" | awk '{print $1}'; }
fail() { printf '%s\n' "$1" >&2; exit 1; }
CURRENT=$(hash "$TARGET")
case "${1:-}" in
    install)
        INPUT=${2:?Supply patched JAR path}
        EXPECTED=${3:?Supply patched JAR SHA256}
        case "$EXPECTED" in *[!a-f0-9]*|'') fail 'Invalid SHA256';; esac
        [ "${#EXPECTED}" -eq 64 ] || fail 'Invalid SHA256 length'
        [ "$(hash "$INPUT")" = "$EXPECTED" ] || fail 'Patched JAR checksum mismatch'
        if [ "$CURRENT" = "$EXPECTED" ]; then printf '%s\n' 'Navigation fix already installed'; exit 0; fi
        case "$CURRENT" in
            "$ORIGINAL") ;;
            *) fail 'Unknown or modified KUAL; refusing replacement';;
        esac
        mkdir -p "$BACKUP"
        if [ ! -f "$BACKUP/original.jar" ]; then
            [ "$CURRENT" = "$ORIGINAL" ] || fail 'Original backup missing'
            cp -p "$TARGET" "$BACKUP/original.jar"
        fi
        [ "$(hash "$BACKUP/original.jar")" = "$ORIGINAL" ] || fail 'Original backup checksum mismatch'
        # Save rollback identity before touching the system file (power-loss safe).
        printf '%s\n' "$EXPECTED" > "$BACKUP/installed.sha256"
        ;;
    rollback)
        INPUT="$BACKUP/original.jar"
        EXPECTED=$ORIGINAL
        [ "$(hash "$INPUT")" = "$ORIGINAL" ] || fail 'Original backup checksum mismatch'
        if [ "$CURRENT" = "$ORIGINAL" ]; then printf '%s\n' 'Original KUAL is already installed'; exit 0; fi
        [ -f "$BACKUP/installed.sha256" ] || fail 'Installed patch identity missing'
        [ "$CURRENT" = "$(cat "$BACKUP/installed.sha256")" ] || fail 'KUAL changed since patching; refusing rollback'
        ;;
    *) fail 'Usage: device-install.sh install PATCHED.jar SHA256 | rollback';;
esac
# Preserve the previous mount mode and leave the root filesystem read-only when
# it was read-only. No reboot hooks, registry changes, or power settings.
WAS_RO=$(awk '$2 == "/" && $4 ~ /(^|,)ro(,|$)/ { found=1 } END { print found+0 }' /proc/mounts)
STAGED=
cleanup() {
    if [ -n "$STAGED" ]; then rm -f "$STAGED"; fi
    if [ "$WAS_RO" -eq 1 ]; then mount -o remount,ro /; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
lipc-set-prop com.lab126.appmgrd stop app://com.mobileread.ixtab.kindlelauncher 2>/dev/null || true
if [ "$WAS_RO" -eq 1 ]; then mount -o remount,rw /; fi
# Keep the old inode linked while the framework still holds its JAR open.
# Otherwise Linux 3.0 may refuse remounting the root filesystem read-only.
INODE_BACKUP="$TARGET.inkpika-$CURRENT"
if [ ! -e "$INODE_BACKUP" ]; then ln "$TARGET" "$INODE_BACKUP"; fi
[ "$(hash "$INODE_BACKUP")" = "$CURRENT" ] || fail 'Inode backup checksum mismatch'
STAGED=$(mktemp "$TARGET.inkpika.XXXXXX")
cp "$INPUT" "$STAGED"
chmod 644 "$STAGED"
chown 0:0 "$STAGED"
[ "$(hash "$STAGED")" = "$EXPECTED" ] || fail 'Staged JAR checksum mismatch'
mv -f "$STAGED" "$TARGET"
STAGED=
sync
[ "$(hash "$TARGET")" = "$EXPECTED" ] || fail 'Installed JAR checksum mismatch'
printf '%s\n' 'KUAL navigation change installed; original backup retained.'
