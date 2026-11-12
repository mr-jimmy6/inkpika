#!/usr/bin/env python3
"""Create a reproducible release from an allowlist; never walk runtime folders."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import tempfile
from zipfile import ZIP_DEFLATED, ZipFile, ZipInfo

ROOT = Path(__file__).resolve().parent.parent
EXTENSION = "extensions/inkpika/"
BINARY = EXTENSION + "bin/inkpika"
METADATA = EXTENSION + "build.json"
FILES = (
    "README.md", "README_ZH.md", "README_DEV.md", "AGENTS.md", "LICENSE", "THIRD_PARTY.md", "CONTRIBUTING.md", "RELEASE_NOTES.md",
    "docs/OBJECT_STORAGE.md", "docs/OBJECT_STORAGE_ZH.md",
    "docs/LICENSE-go.txt", "docs/LICENSE-apache-2.0.txt",
    "docs/LICENSE-mpl-2.0.txt", "docs/DEVICE_VALIDATION.md", "docs/DEVICE_COMPATIBILITY.md", "docs/DEVELOPMENT.md", "docs/COMPATIBILITY.md", "docs/LOCALIZATION.md",
    "docs/LICENSE-qrcode.txt", "docs/LICENSE-gofakes3.txt", "docs/KUAL_NAVIGATION.md", "docs/images/feature-overview-zh.png",
    "docs/images/feature-overview-en.png",
    "docs/images/best-practices-zh.png", "docs/images/best-practices-en.png",
) + tuple(EXTENSION + name for name in (
    "config.xml", "menu.json", "config.example.json", "config.aliyun.example.json",
    "config.qingstor.example.json", "config.s3.example.json", "config.azure.example.json", "start.sh", "stop.sh", "service.sh", "sync.sh", "menu.sh",
    "register-ui.sh", "unregister-ui.sh", "ui/config.xml", "ui/index.html",
    "settings-start.sh", "settings-stop.sh", "settings-service.sh", "open-ui.sh", "transfer-start.sh", "transfer-stop.sh", "transfer-service.sh", "transfer.example.json",
))
TEMPLATES = {EXTENSION + "config.xml", EXTENSION + "ui/config.xml"}


def release_version(root):
    version = (root / "internal/buildinfo/VERSION").read_text().strip()
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:\.dev|-[A-Za-z0-9.-]+)?", version):
        raise ValueError("Invalid release version")
    return version


PACKAGE_NAME = f"inkpika-{release_version(ROOT)}-armv5.zip"


def build_time():
    epoch = os.environ.get("SOURCE_DATE_EPOCH")
    instant = datetime.fromtimestamp(int(epoch), timezone.utc) if epoch is not None else datetime.now(timezone.utc)
    return instant.strftime("%Y-%m-%dT%H:%M:%SZ")


def validate_build_time(value):
    datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ")
    if not re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", value):
        raise ValueError("Package time must be an RFC3339 UTC timestamp")
    return value


def package(root, binary, output_dir, packaged_at):
    version = release_version(root)
    validate_build_time(packaged_at)
    header = binary.read_bytes()[:20]
    if header[:6] != b"\x7fELF\x01\x01" or header[18:20] != b"\x28\x00":
        raise ValueError("Release binary must be a 32-bit little-endian ARM ELF")
    output_dir.mkdir(parents=True, exist_ok=True)
    output = output_dir / f"inkpika-{version}-armv5.zip"
    fd, temporary = tempfile.mkstemp(prefix=".package-", suffix=".zip", dir=output_dir)
    os.close(fd)
    try:
        with ZipFile(temporary, "w", ZIP_DEFLATED, compresslevel=9) as archive:
            for name in sorted((*FILES, BINARY, METADATA)):
                if name == METADATA:
                    data = (json.dumps({"version": version, "packaged_at": packaged_at}, sort_keys=True) + "\n").encode()
                else:
                    source = binary if name == BINARY else root / name
                    if source.is_symlink() or not source.is_file():
                        raise ValueError(f"Release input must be a regular file: {name}")
                    data = source.read_bytes()
                if name in TEMPLATES:
                    data = data.replace(b"@VERSION@", version.encode("ascii"))
                info = ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                mode = 0o755 if name == BINARY or name.endswith(".sh") else 0o644
                info.external_attr = (stat.S_IFREG | mode) << 16
                info.compress_type = ZIP_DEFLATED
                archive.writestr(info, data, compresslevel=9)
        os.replace(temporary, output)
    finally:
        Path(temporary).unlink(missing_ok=True)
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    output.with_suffix(".zip.sha256").write_text(f"{digest}  {output.name}\n")
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--packaged-at")
    parser.add_argument("--print-build-time", action="store_true")
    args = parser.parse_args()
    if args.print_build_time:
        print(build_time())
        return
    if args.binary is None or args.packaged_at is None:
        parser.error("--binary and --packaged-at are required when creating a package")
    output = package(ROOT, args.binary, ROOT / "dist", args.packaged_at)
    print(f"Built {output.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
