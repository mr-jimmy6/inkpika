#!/usr/bin/env python3
"""Build or reuse a release package and install InkPika over SSH."""
import argparse
import hashlib
import io
from pathlib import Path
import re
import shlex
import stat
import subprocess
import sys
import tarfile
import tempfile
from zipfile import BadZipFile, ZipFile

import package as release

ROOT = Path(__file__).resolve().parent.parent
DESTINATION = "/mnt/us/extensions/inkpika"

# A single SSH connection holds the deployment lock through upload and install.
# Use tar over SSH: old Kindle SSH servers need neither SFTP nor modern scp.
REMOTE_COMMAND = r"""set -eu
umask 077
for tool in tar sha256sum mktemp readlink; do
    command -v "$tool" >/dev/null || exit 1
done
for directory in /mnt /mnt/us /mnt/us/extensions; do
    [ -d "$directory" ] && [ ! -L "$directory" ] || {
        echo 'Expected a real Kindle extensions directory.' >&2; exit 1;
    }
done
LOCK=/mnt/us/.inkpika-deploy.lock
mkdir "$LOCK" 2>/dev/null || {
    echo 'Deployment lock exists; check for another deployment before removing it.' >&2
    exit 1
}
STAGE=
cleanup() {
    result=$?
    trap - 0 HUP INT TERM
    if [ -n "$STAGE" ]; then
        if [ -f "$STAGE/keep" ]; then
            printf 'Deployment backup/recovery directory: %s\n' "$STAGE"
        else
            rm -rf "$STAGE"
        fi
    fi
    rmdir "$LOCK"
    exit "$result"
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
STAGE=$(mktemp -d /mnt/us/.inkpika-deploy.XXXXXX)
tar -xf - -C "$STAGE"
sh "$STAGE/install.sh" "$STAGE"
"""


def ssh_command(args):
    command = ["ssh", "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
               "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=5",
               "-o", "ServerAliveCountMax=3"]
    if args.port:
        command += ["-p", str(args.port)]
    if args.identity:
        command += ["-i", str(args.identity.expanduser())]
    return command + [args.host, REMOTE_COMMAND]


def add_file(archive, name, data, mode=0o644):
    info = tarfile.TarInfo(name)
    info.size = len(data)
    info.mode = mode
    archive.addfile(info, io.BytesIO(data))


def prepare_payload(package, stream):
    """Verify a release and copy only its allowlisted extension files."""
    digest = hashlib.sha256(package.read_bytes()).hexdigest()
    if package.with_suffix(".zip.sha256").read_text().strip() != f"{digest}  {package.name}":
        raise ValueError("Release ZIP checksum mismatch")
    expected = {*release.FILES, release.BINARY, release.METADATA}
    with ZipFile(package) as source, tarfile.open(fileobj=stream, mode="w") as target:
        names = source.namelist()
        if len(names) != len(expected) or set(names) != expected:
            raise ValueError("Release ZIP does not match the packaging allowlist")
        checksums = []
        for name in sorted(expected):
            info = source.getinfo(name)
            if not stat.S_ISREG(info.external_attr >> 16):
                raise ValueError(f"Release entry must be a regular file: {name}")
            if not name.startswith(release.EXTENSION):
                continue
            relative = name[len(release.EXTENSION):]
            data = source.read(name)
            if name == release.BINARY:
                if data[:6] != b"\x7fELF\x01\x01" or data[18:20] != b"\x28\x00":
                    raise ValueError("Release binary must be a 32-bit little-endian ARM ELF")
            mode = 0o755 if name == release.BINARY or name.endswith(".sh") else 0o644
            add_file(target, "payload/" + relative, data, mode)
            checksums.append(f"{hashlib.sha256(data).hexdigest()}  {relative}\n")
        add_file(target, "SHA256SUMS", "".join(checksums).encode("ascii"))
        add_file(target, "install.sh", (ROOT / "scripts/deploy-device.sh").read_bytes(), 0o755)
    stream.seek(0)


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("host", help="SSH alias or user@host (for example kindle-dev)")
    parser.add_argument("-p", "--port", type=int, help="SSH port; defaults to SSH configuration")
    parser.add_argument("-i", "--identity", type=Path, help="SSH private key on this computer")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--skip-check", action="store_true", help="skip check.sh; always rebuild the package")
    mode.add_argument("--package", type=Path, metavar="ZIP",
                      help="install an existing ZIP with its .zip.sha256 file; skip checking and building source")
    parser.add_argument("--dry-run", action="store_true", help="print the plan without building or connecting")
    args = parser.parse_args(argv)
    if not re.fullmatch(r"[A-Za-z0-9\[][A-Za-z0-9_.@:%\[\]-]*", args.host):
        parser.error("host must be an SSH alias or user@host without whitespace or shell syntax")
    if args.port is not None and not 1 <= args.port <= 65535:
        parser.error("port must be between 1 and 65535")
    return args


def main(argv=None):
    args = parse_args(argv)
    if args.package:
        package = args.package.expanduser().resolve()
        steps = []
    else:
        package = ROOT / "dist" / release.PACKAGE_NAME
        steps = ([] if args.skip_check else ["check.sh"]) + ["build.sh"]
    command = ssh_command(args)
    print("Local steps: " + (" -> ".join(steps) if steps else "reuse existing package; no source check or build"), flush=True)
    print(f"Release: {package}", flush=True)
    print(f"Install to: {args.host}:{DESTINATION}", flush=True)
    print("Services will be stopped and left stopped; open KUAL to start them.", flush=True)
    if args.dry_run:
        print("SSH: " + shlex.join(command[:-1]) + " <upload and install>")
        print("No build or SSH connection performed.")
        return 0
    for step in steps:
        subprocess.run(["sh", str(ROOT / "scripts" / step)], cwd=ROOT, check=True)
    with tempfile.TemporaryFile() as stream:
        prepare_payload(package, stream)
        subprocess.run(command, stdin=stream, check=True)
    print("Deployment complete. Open KUAL -> InkPika on the device.")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, BadZipFile, subprocess.CalledProcessError) as error:
        # Do not print the full SSH command, remote output or any device config.
        if isinstance(error, subprocess.CalledProcessError):
            print(f"Deployment failed: {Path(error.cmd[0]).name} exited with {error.returncode}.", file=sys.stderr)
        else:
            print(f"Deployment failed: {error}", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        print("Deployment interrupted; check the device before retrying.", file=sys.stderr)
        sys.exit(130)
