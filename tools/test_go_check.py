"""Verify ordinary/race target separation, bounded scheduling and failure propagation."""
import argparse
import csv
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from go_check import ROOT, test_command


def arguments(**overrides):
    return argparse.Namespace(**dict(dict(race=False, run=None, count=None, profile=None, packages=[]), **overrides))


class GoCheckTest(unittest.TestCase):
    def test_ordinary_tests_keep_go_cache_and_no_race_instrumentation(self):
        command, environment = test_command(arguments(), {"PATH": "fixture"})
        self.assertEqual(command, ["go", "test", "./..."])
        self.assertNotIn("GOMAXPROCS", environment)

    @unittest.skipUnless(os.name == "posix", "POSIX nice scheduling")
    def test_race_tests_are_low_priority_and_bounded(self):
        with patch("go_check.shutil.which", return_value="/usr/bin/nice"):
            command, environment = test_command(arguments(race=True), {})
        self.assertEqual(command, ["/usr/bin/nice", "-n", "15", "go", "test", "-race", "-p", "1", "./..."])
        self.assertEqual(environment["GOMAXPROCS"], "2")
        _, environment = test_command(arguments(race=True), {"GOMAXPROCS": "1"})
        self.assertEqual(environment["GOMAXPROCS"], "1")

    def test_make_keeps_race_explicit_and_orders_the_full_gate(self):
        def dry(target):
            return subprocess.check_output(["make", "-n", target], cwd=ROOT, text=True)
        everyday, race, full = dry("check"), dry("race-check"), dry("check-full")
        self.assertNotIn("go_check.py --race", everyday)
        self.assertIn("go_check.py --race", race)
        self.assertLess(full.index("tools/go_check.py\n"), full.index("tools/go_check.py --race"))

    @unittest.skipUnless(os.name == "posix", "POSIX executable fixture")
    def test_failure_reaches_make_and_history_without_arguments(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            fake_go = folder / "go"
            fake_go.write_text("#!" + sys.executable + "\nimport json, os\nprint(json.dumps({'nice':os.getpriority(os.PRIO_PROCESS, 0),'cpus':os.environ.get('GOMAXPROCS')}))\nraise SystemExit(7)\n")
            fake_go.chmod(0o700)
            history = folder / "history.csv"
            environment = dict(os.environ, PATH=str(folder) + os.pathsep + os.environ["PATH"], TELEPORT_TOOL_HISTORY=str(history))
            environment.pop("GOMAXPROCS", None)
            result = subprocess.run([sys.executable, "-B", str(ROOT / "tools/go_check.py"), "--race", "--run", "private-test-pattern"], env=environment, capture_output=True, text=True)
            self.assertEqual(result.returncode, 7)
            scheduling = json.loads(result.stdout.splitlines()[1])
            self.assertGreaterEqual(scheduling["nice"], 15)
            self.assertEqual(scheduling["cpus"], "2")
            with history.open(newline="") as source:
                rows = list(csv.DictReader(source))
            self.assertEqual(rows[0]["outcome"], "failure")
            self.assertEqual(rows[0]["exit_code"], "7")
            self.assertNotIn("private-test-pattern", history.read_text())
            result = subprocess.run(["make", "race-check"], cwd=ROOT, env=environment, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
