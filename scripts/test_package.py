"""Release integrity checks; fixtures never use local credentials or a device."""
import hashlib
import json
from unittest.mock import patch
import os
from pathlib import Path
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET
from zipfile import ZipFile

import package as release


class PackageTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        for name in (*release.FILES, "internal/buildinfo/VERSION"):
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(release.ROOT / name, target)
        self.binary = self.root / "inkpika"
        self.binary.write_bytes(b"\x7fELF\x01\x01" + bytes(12) + b"\x28\x00" + b"fixture")
        self.output = self.root / "dist"

    def build(self):
        return release.package(self.root, self.binary, self.output, "2026-11-09T01:02:03Z")

    def test_allowlist_versions_permissions_and_checksum(self):
        for name in ("config.json", "transfer.json", "transfer-state/launch.url", "settings-state/launch.url", "config.json.previous", "config.json.lock", "state/jobs.json", "ui/launch.js", "ui/service", "private.sh"):
            target = self.root / release.EXTENSION / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text("PRIVATE-CANARY")
        output = self.build()
        with ZipFile(output) as archive:
            self.assertEqual(set(archive.namelist()), {*release.FILES, release.BINARY, release.METADATA})
            self.assertEqual(json.loads(archive.read(release.METADATA)), {"version": release.release_version(self.root), "packaged_at": "2026-11-09T01:02:03Z"})
            for info in archive.infolist():
                data = archive.read(info)
                self.assertNotIn(b"PRIVATE-CANARY", data)
                expected = 0o755 if info.filename == release.BINARY or info.filename.endswith(".sh") else 0o644
                self.assertEqual((info.external_attr >> 16) & 0o777, expected)
            for name in release.TEMPLATES:
                xml = ET.fromstring(archive.read(name))
                version = xml.attrib.get("version") or xml.findtext("information/version")
                self.assertEqual(version, release.release_version(self.root))
        digest = hashlib.sha256(output.read_bytes()).hexdigest()
        self.assertEqual(output.with_suffix(".zip.sha256").read_text(), f"{digest}  {output.name}\n")

    def test_source_mtime_and_mode_do_not_change_archive(self):
        output = self.build()
        before = output.read_bytes()
        for name in release.FILES:
            source = self.root / name
            os.utime(source, (1700000000, 1700000000))
            source.chmod(0o777)
        self.assertEqual(self.build().read_bytes(), before)

    def test_package_is_overwritten_with_current_metadata(self):
        first = self.build()
        self.binary.write_bytes(self.binary.read_bytes() + b"changed")
        second = release.package(self.root, self.binary, self.output, "2026-11-09T02:03:04Z")
        self.assertEqual(first, second)
        self.assertEqual(second.name, "inkpika-0.0.1.dev-armv5.zip")
        self.assertEqual(list(self.output.glob("*.zip")), [second])
        with ZipFile(second) as archive:
            self.assertEqual(json.loads(archive.read(release.METADATA))["packaged_at"], "2026-11-09T02:03:04Z")
            self.assertEqual(archive.read(release.BINARY), self.binary.read_bytes())

    def test_reproducible_build_time_and_invalid_timestamp(self):
        with patch.dict(os.environ, {"SOURCE_DATE_EPOCH": "0"}):
            self.assertEqual(release.build_time(), "1970-01-01T00:00:00Z")
        first = self.build().read_bytes()
        with self.assertRaises(ValueError):
            release.package(self.root, self.binary, self.output, "not a time")
        self.assertEqual((self.output / release.PACKAGE_NAME).read_bytes(), first)

    def test_bad_input_preserves_previous_release(self):
        output = self.build()
        before = output.read_bytes()
        (self.root / "README.md").unlink()
        with self.assertRaises(ValueError):
            self.build()
        self.assertEqual(output.read_bytes(), before)
        self.assertEqual(list(self.output.glob(".package-*")), [])

    def test_symlink_and_wrong_architecture_rejected(self):
        readme = self.root / "README.md"
        readme.unlink()
        readme.symlink_to(self.root / "LICENSE")
        with self.assertRaises(ValueError):
            self.build()
        readme.unlink()
        readme.write_text("readme")
        self.binary.write_bytes(b"not an ARM executable")
        with self.assertRaises(ValueError):
            self.build()


if __name__ == "__main__":
    unittest.main()
