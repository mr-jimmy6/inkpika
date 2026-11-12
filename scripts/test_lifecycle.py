"""Execute the shell entry points with isolated process/UI command doubles."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent / "extensions/inkpika"


class LifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for name in ("bin", "ui", "state", "commands"):
            (self.root / name).mkdir()
        self.events = self.root / "events"
        self.env = dict(os.environ, INKPIKA_TEST_EVENTS=str(self.events),
                        INKPIKA_TEST_STATE=str(self.root / "state"),
                        PATH=str(self.root / "commands") + os.pathsep + os.environ["PATH"])
        self.script("commands/lipc-set-prop", 'echo ui-stop >> "$INKPIKA_TEST_EVENTS"')
        self.script("commands/sqlite3", 'echo registry >> "$INKPIKA_TEST_EVENTS"')
        self.script("bin/inkpika", 'echo "$INKPIKA_TEST_STATE"')
        self.script("menu.sh", 'echo menu-refresh >> "$INKPIKA_TEST_EVENTS"')

    def script(self, name, body):
        p = self.root / name
        p.write_text("#!/bin/sh\n" + body + "\n")
        p.chmod(0o755)

    def run_script(self, name):
        return subprocess.run(["sh", str(self.root / name)], env=self.env,
                              capture_output=True, text=True)

    def test_cloud_stop_preserves_other_service_windows(self):
        shutil.copyfile(ROOT / "stop.sh", self.root / "stop.sh")
        for service in ("settings", "transfer", "cloud", ""):
            with self.subTest(service=service):
                self.events.write_text("")
                (self.root / "ui/service").write_text(service)
                (self.root / "ui/launch.js").write_text("private launch fixture")
                result = self.run_script("stop.sh")
                self.assertEqual(result.returncode, 0, result.stderr)
                preserved = service in ("settings", "transfer")
                self.assertEqual((self.root / "ui/launch.js").exists(), preserved)
                self.assertEqual("ui-stop" not in self.events.read_text(), preserved)

    def test_unregister_stops_all_services_before_registration_removal(self):
        shutil.copyfile(ROOT / "unregister-ui.sh", self.root / "unregister-ui.sh")
        for name in ("settings-stop", "transfer-stop", "stop"):
            self.script(name + ".sh", 'echo ' + name + ' >> "$INKPIKA_TEST_EVENTS"')
        for configured in (False, True):
            with self.subTest(configured=configured):
                self.events.write_text("")
                if configured:
                    (self.root / "config.json").write_text("{}")
                result = self.run_script("unregister-ui.sh")
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = self.events.read_text().splitlines()
                expected = ["settings-stop", "transfer-stop"] + (["stop"] if configured else [])
                self.assertEqual(lines[:len(expected)], expected)
                self.assertTrue(all(line == "registry" for line in lines[len(expected):]))

    def test_unregister_aborts_when_service_cannot_stop(self):
        shutil.copyfile(ROOT / "unregister-ui.sh", self.root / "unregister-ui.sh")
        self.script("settings-stop.sh", 'echo failed-stop >> "$INKPIKA_TEST_EVENTS"\nexit 1')
        result = self.run_script("unregister-ui.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("registry", self.events.read_text())


if __name__ == "__main__":
    unittest.main()
