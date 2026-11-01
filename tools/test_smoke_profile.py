"""Verify safe, correlated stage timings and browser runtime selection."""
import csv
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from browser_setup import runtime_for_args
from smoke_profile import Profile, record_timeout


class ProfileTest(unittest.TestCase):
    def test_failure_records_counts_and_no_action_values(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            path = folder / "stages.csv"
            with patch.dict(os.environ, TELEPORT_TOOL_STAGES=str(path), TELEPORT_TOOL_RUN_ID="parent-run"):
                profile = Profile("sqlite/full-persistent", folder)
                profile.start("navigation")
                profile.call("browser.fill", lambda: "private-password")
                with self.assertRaises(ValueError):
                    profile.call("browser.fill", lambda: (_ for _ in ()).throw(ValueError("private error")))
                profile.finish("failure")
                profile.finish()
            with path.open() as source:
                rows = list(csv.DictReader(source))
            self.assertEqual(len(rows), 2)
            self.assertEqual(rows[1]["calls"], "2")
            self.assertEqual({row["run_id"] for row in rows}, {"parent-run"})
            self.assertEqual({row["outcome"] for row in rows}, {"failure"})
            self.assertGreaterEqual(float(rows[0]["duration_seconds"]), float(rows[1]["duration_seconds"]))
            self.assertNotIn("private", path.read_text())

    def test_api_and_cli_paths_do_not_require_optional_client(self):
        for arguments in ([], ["--browser", "--help"], ["--browser", "--browser-driver=cli"], ["--browser", "--browser-driver", "cli"]):
            self.assertIsNone(runtime_for_args(arguments))
        with patch("browser_setup.browser_python", return_value="pinned-python"):
            self.assertEqual(runtime_for_args(["--browser"]), "pinned-python")

    def test_supervisor_recovers_the_interrupted_stage(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            path = folder / "stages.csv"
            with patch.dict(os.environ, TELEPORT_TOOL_STAGES=str(path)):
                Profile("sqlite/full-persistent", folder).start("workflow")
                record_timeout(folder)
                record_timeout(folder)
            with path.open() as source:
                rows = list(csv.DictReader(source))
            self.assertEqual(len(rows), 1)
            self.assertEqual((rows[0]["stage"], rows[0]["outcome"]), ("workflow", "timeout"))


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
