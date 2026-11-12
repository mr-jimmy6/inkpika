"""Exercise SSH deployment with temporary files and command doubles, never a Kindle."""
import hashlib
import io
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
from zipfile import ZipFile, ZipInfo

import deploy
import package as release

BINARY_DATA = b"\x7fELF\x01\x01" + bytes(12) + b"\x28\x00" + b"fixture"


class DeployTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="bridge-deploy-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.mount = self.root / "mnt/us"
        self.app = self.mount / "extensions/inkpika"
        self.app.mkdir(parents=True)
        self.stage = self.mount / ".inkpika-deploy.fixture"
        self.stage.mkdir()
        self.package = self.root / "release.zip"
        self.events = self.root / "events"
        self.commands = self.root / "commands"
        self.commands.mkdir()
        self.env = dict(os.environ, INKPIKA_TEST_EVENTS=str(self.events),
                        PATH=str(self.commands) + os.pathsep + os.environ["PATH"])
        # macOS has shasum instead of sha256sum; emulate the BusyBox interface.
        if not shutil.which("sha256sum"):
            self.script(self.commands / "sha256sum", 'exec shasum -a 256 "$@"')
        self.script(self.commands / "sync", "exit 0")

    def script(self, path, body):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("#!/bin/sh\nset -eu\n" + body + "\n")
        path.chmod(0o755)

    def make_package(self, extra=None, binary=BINARY_DATA):
        with ZipFile(self.package, "w") as archive:
            for name in sorted({*release.FILES, release.BINARY, release.METADATA} | set(extra or [])):
                info = ZipInfo(name)
                info.external_attr = (stat.S_IFREG | 0o644) << 16
                archive.writestr(info, binary if name == release.BINARY else "new " + name)
        self.sign_package()

    def sign_package(self):
        digest = hashlib.sha256(self.package.read_bytes()).hexdigest()
        self.package.with_suffix(".zip.sha256").write_text(f"{digest}  {self.package.name}\n")

    def payload(self):
        self.make_package()
        stream = io.BytesIO()
        deploy.prepare_payload(self.package, stream)
        return stream

    def unpack(self):
        with tarfile.open(fileobj=self.payload()) as archive:
            # The payload's names have already been checked against the release allowlist.
            for info in archive.getmembers():
                path = self.stage / info.name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(archive.extractfile(info).read())
                path.chmod(info.mode)

    def existing_install(self, configured=True):
        for relative in ("bin/inkpika", "menu.json", "ui/index.html"):
            path = self.app / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("old " + relative)
        for name in ("settings-stop", "transfer-stop", "stop"):
            self.script(self.app / (name + ".sh"), f'echo {name} >> "$INKPIKA_TEST_EVENTS"')
        private = ["transfer.json", "state/jobs.json", "settings-state/private", "ui/custom.txt"]
        if configured:
            private.append("config.json")
        for relative in private:
            path = self.app / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("PRIVATE-CANARY " + relative)
        book = self.mount / "documents/book.azw3"
        book.parent.mkdir(parents=True, exist_ok=True)
        book.write_text("PRIVATE-BOOK")
        return {path: path.read_bytes() for path in self.app.rglob("*") if path.is_file()} | {book: book.read_bytes()}

    def run_installer(self):
        script = (deploy.ROOT / "scripts/deploy-device.sh").read_text()
        script = script.replace("/mnt/us", str(self.mount))
        # Do not inspect real host processes during isolated tests.
        script = script.replace("/proc/[0-9]*/exe", str(self.root / "proc/[0-9]*/exe"))
        return subprocess.run(["sh", "-c", script, "install.sh", str(self.stage)],
                              env=self.env, capture_output=True, text=True)

    def test_payload_contains_only_release_extension_and_installer(self):
        with tarfile.open(fileobj=self.payload()) as archive:
            expected = {"payload/" + name[len(release.EXTENSION):]
                        for name in (*release.FILES, release.BINARY, release.METADATA)
                        if name.startswith(release.EXTENSION)} | {"SHA256SUMS", "install.sh"}
            self.assertEqual(set(archive.getnames()), expected)
            self.assertEqual(archive.getmember("payload/bin/inkpika").mode, 0o755)
            self.assertEqual(archive.getmember("payload/start.sh").mode, 0o755)
            self.assertNotIn("payload/config.json", expected)

    def test_corrupt_or_non_allowlisted_package_is_rejected(self):
        self.make_package()
        self.package.write_bytes(self.package.read_bytes() + b"corrupt")
        with self.assertRaisesRegex(ValueError, "checksum"):
            deploy.prepare_payload(self.package, io.BytesIO())
        for extra in ("extensions/inkpika/config.json", "../escape"):
            self.make_package([extra])
            with self.assertRaisesRegex(ValueError, "allowlist"):
                deploy.prepare_payload(self.package, io.BytesIO())

    def test_install_preserves_private_files_and_backs_up_program(self):
        for configured in (False, True):
            with self.subTest(configured=configured):
                before = self.existing_install(configured)
                self.unpack()
                result = self.run_installer()
                self.assertEqual(result.returncode, 0, result.stderr)
                expected = ["settings-stop", "transfer-stop"] + (["stop"] if configured else [])
                self.assertEqual(self.events.read_text().splitlines(), expected)
                self.events.unlink()
                self.assertEqual((self.app / "bin/inkpika").read_bytes(), BINARY_DATA)
                self.assertEqual((self.app / "bin/inkpika").stat().st_mode & 0o777, 0o755)
                self.assertEqual((self.app / "config.example.json").stat().st_mode & 0o777, 0o644)
                self.assertEqual((self.stage / "backup/bin/inkpika").read_text(), "old bin/inkpika")
                for path, data in before.items():
                    if data.startswith(b"PRIVATE"):
                        self.assertEqual(path.read_bytes(), data)
                self.assertTrue((self.stage / "keep").exists())
                self.assertFalse((self.stage / "payload").exists())

    def test_first_install_does_not_require_cloud_configuration(self):
        self.app.rmdir()
        self.unpack()
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.events.exists())
        self.assertFalse((self.app / "config.json").exists())
        self.assertTrue((self.app / "bin/inkpika").exists())
        for relative in (".", "bin", "ui"):
            self.assertEqual((self.app / relative).stat().st_mode & 0o777, 0o755)

    def test_bad_upload_or_symlink_aborts_before_stop(self):
        before = self.existing_install()
        self.unpack()
        binary = self.stage / "payload/bin/inkpika"
        binary.write_text("corrupt upload")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists())
        for path, data in before.items():
            self.assertEqual(path.read_bytes(), data)
        self.unpack()
        (self.app / "ui/index.html").unlink()
        (self.app / "ui/index.html").symlink_to(self.root / "outside")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists())
        self.assertFalse((self.root / "outside").exists())

    def test_stop_failure_or_lingering_process_prevents_replacement(self):
        self.existing_install()
        self.unpack()
        self.script(self.app / "settings-stop.sh", "exit 1")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.app / "bin/inkpika").read_text(), "old bin/inkpika")
        self.script(self.app / "settings-stop.sh", "exit 0")
        process = self.root / "proc/123/exe"
        process.parent.mkdir(parents=True)
        process.symlink_to(self.app / "bin/inkpika")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("still running", result.stderr)
        self.assertEqual((self.app / "bin/inkpika").read_text(), "old bin/inkpika")

    def test_failed_replacement_rolls_back_existing_and_added_files(self):
        before = self.existing_install()
        self.unpack()
        real_mv = shutil.which("mv")
        self.script(self.commands / "mv", 'case "$2" in\n'
                    '    */payload/menu.json) exit 1 ;;\nesac\n'
                    f'exec "{real_mv}" "$@"')
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("previous program files restored", result.stderr)
        for path, data in before.items():
            self.assertEqual(path.read_bytes(), data)
        self.assertFalse((self.app / "config.example.json").exists())
        self.assertTrue((self.stage / "keep").exists())

    def test_remote_upload_failure_releases_lock_without_touching_install(self):
        before = self.existing_install()
        remote = deploy.REMOTE_COMMAND.replace("/mnt/us", str(self.mount))
        remote = remote.replace("in /mnt ", "in " + str(self.root / "mnt") + " ")
        result = subprocess.run(["sh", "-c", remote], input=b"not a tar archive",
                                env=self.env, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.mount / ".inkpika-deploy.lock").exists())
        for path, data in before.items():
            self.assertEqual(path.read_bytes(), data)

    def test_full_remote_upload_install_and_lock(self):
        remote = deploy.REMOTE_COMMAND.replace("/mnt/us", str(self.mount))
        remote = remote.replace("in /mnt ", "in " + str(self.root / "mnt") + " ")
        installer = (deploy.ROOT / "scripts/deploy-device.sh").read_bytes()
        installer = installer.replace(b"/mnt/us", str(self.mount).encode())
        installer = installer.replace(b"/proc/[0-9]*/exe", str(self.root / "proc/[0-9]*/exe").encode())
        actual_read = Path.read_bytes

        def read_bytes(path):
            if path == deploy.ROOT / "scripts/deploy-device.sh":
                return installer
            return actual_read(path)

        with patch.object(Path, "read_bytes", read_bytes):
            data = self.payload().getvalue()
        lock = self.mount / ".inkpika-deploy.lock"
        lock.mkdir()
        result = subprocess.run(["sh", "-c", remote], input=data, env=self.env, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"Deployment lock exists", result.stderr)
        self.assertFalse((self.app / "bin/inkpika").exists())
        lock.rmdir()
        result = subprocess.run(["sh", "-c", remote], input=data, env=self.env, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(b"Installed files verified", result.stdout)
        self.assertIn(b"backup/recovery directory", result.stdout)
        self.assertFalse(lock.exists())
        self.assertTrue((self.app / "bin/inkpika").exists())

    def test_post_install_checksum_failure_restores_program(self):
        before = self.existing_install()
        self.unpack()
        if shutil.which("sha256sum"):
            checksum = '"' + shutil.which("sha256sum") + '"'
        else:
            checksum = 'shasum -a 256'
        self.script(self.commands / "sha256sum", f'case "$PWD" in\n'
                    '    */extensions/inkpika) exit 1 ;;\nesac\n'
                    f'exec {checksum} "$@"')
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("previous program files restored", result.stderr)
        for path, data in before.items():
            self.assertEqual(path.read_bytes(), data)

    def test_cli_plan_build_order_and_ssh_options(self):
        with patch.object(deploy.subprocess, "run") as run, patch("builtins.print"):
            self.assertEqual(deploy.main(["kindle-dev", "--dry-run"]), 0)
            run.assert_not_called()
        with patch.object(deploy.subprocess, "run") as run, \
                patch.object(deploy, "prepare_payload") as prepare, patch("builtins.print"):
            deploy.main(["root@kindle", "-p", "2222", "-i", "/tmp/key with spaces"])
            calls = run.call_args_list
            self.assertEqual(Path(calls[0].args[0][1]).name, "check.sh")
            self.assertEqual(Path(calls[1].args[0][1]).name, "build.sh")
            command = calls[2].args[0]
            self.assertEqual(command[-2], "root@kindle")
            self.assertIn("StrictHostKeyChecking=yes", command)
            self.assertIn("/tmp/key with spaces", command)
            self.assertEqual(command[command.index("-p") + 1], "2222")
            prepare.assert_called_once()
        with patch.object(deploy.subprocess, "run", side_effect=subprocess.CalledProcessError(1, ["sh"])) as run, \
                patch("builtins.print"):
            with self.assertRaises(subprocess.CalledProcessError):
                deploy.main(["kindle-dev"])
            self.assertEqual(run.call_count, 1)
        with patch.object(deploy.subprocess, "run") as run, \
                patch.object(deploy, "prepare_payload"), patch("builtins.print"):
            deploy.main(["kindle-dev", "--skip-check"])
            self.assertEqual(Path(run.call_args_list[0].args[0][1]).name, "build.sh")
            self.assertEqual(run.call_count, 2)

    def test_existing_package_skips_build_and_preserves_archive(self):
        # Exercise an external archive path with spaces, relative to the caller.
        self.package = self.root / "already built.zip"
        self.make_package()
        before = self.package.read_bytes()
        checksum_before = self.package.with_suffix(".zip.sha256").read_bytes()
        previous_cwd = Path.cwd()
        try:
            os.chdir(self.root)
            with patch.object(deploy.subprocess, "run") as run, \
                    patch.object(release, "release_version", side_effect=AssertionError("Unexpected build version lookup")), \
                    patch("builtins.print"):
                self.assertEqual(deploy.main(["kindle-dev", "--package", self.package.name]), 0)
                run.assert_called_once()
                self.assertEqual(run.call_args.args[0][0], "ssh")
                self.assertEqual(run.call_args.args[0][-2], "kindle-dev")
        finally:
            os.chdir(previous_cwd)
        self.assertEqual(self.package.read_bytes(), before)
        self.assertEqual(self.package.with_suffix(".zip.sha256").read_bytes(), checksum_before)
        with patch.object(deploy.subprocess, "run") as run, patch("builtins.print"):
            self.assertEqual(deploy.main(["kindle-dev", "--package", str(self.package), "--dry-run"]), 0)
            run.assert_not_called()

    def test_invalid_existing_package_never_connects(self):
        for condition in ("missing", "missing checksum", "bad checksum", "wrong architecture", "extra file"):
            with self.subTest(condition=condition):
                self.make_package()
                if condition == "missing":
                    self.package.unlink()
                elif condition == "missing checksum":
                    self.package.with_suffix(".zip.sha256").unlink()
                elif condition == "bad checksum":
                    self.package.with_suffix(".zip.sha256").write_text("invalid")
                elif condition == "wrong architecture":
                    self.make_package(binary=b"not an ARM executable")
                else:
                    self.make_package(["extensions/inkpika/config.json"])
                with patch.object(deploy.subprocess, "run") as run, patch("builtins.print"):
                    with self.assertRaises((ValueError, FileNotFoundError)):
                        deploy.main(["kindle-dev", "--package", str(self.package)])
                    run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
