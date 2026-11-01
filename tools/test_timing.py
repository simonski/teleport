"""Verify timing reports with real CSV files and an independently launched CLI."""
import csv
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from tool_history import FIELDS, ROOT, run


class TimingTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="teleport-timing-")
        self.addCleanup(self.temporary.cleanup)
        self.folder = Path(self.temporary.name)
        self.history = self.folder / "source.csv"

    def write(self, rows, fields=FIELDS):
        with self.history.open("w", newline="") as output:
            writer = csv.writer(output)
            writer.writerow(fields)
            writer.writerows(rows)

    def invoke(self, default=False):
        path = self.history if default else self.folder / "calls.csv"
        return subprocess.run([sys.executable, "-B", str(ROOT / "tool/timing.py"), *([] if default else [str(self.history)])],
                              cwd=self.folder, env=dict(os.environ, TELEPORT_TOOL_HISTORY=str(path)),
                              capture_output=True, text=True, timeout=10)

    def test_prints_every_invocation_with_seconds_and_csv_quoting(self):
        self.write([
            ["one", "tools/build.py", "2026-11-01T12:00:00Z", "2026-11-01T12:00:02Z", "2.125", "success", "0", ""],
            ["two", "tools/example, fixture.py", "2026-11-01T12:01:00Z", "2026-11-01T12:01:00Z", "0.012345", "failure", "3", "SystemExit"],
        ])
        result = self.invoke()
        self.assertEqual(0, result.returncode, result.stderr)
        lines = result.stdout.splitlines()
        self.assertEqual(2, len(lines))
        self.assertIn("tools/build.py  success  exit=0  2.125000 seconds", lines[0])
        self.assertIn("tools/example, fixture.py  failure  exit=3  0.012345 seconds", lines[1])

    def test_default_history_works_outside_the_repository_and_logs_itself(self):
        self.write([["one", "tools/sbom.py", "start", "end", "1", "handoff", "", ""]])
        result = self.invoke(default=True)
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("handoff  exit=-  1.000000 seconds", result.stdout)
        with self.history.open(newline="") as source:
            rows = list(csv.DictReader(source))
        self.assertEqual(2, len(rows))
        self.assertEqual("tool/timing.py", rows[-1]["tool"])

    def test_rejects_missing_files_bad_headers_and_nonfinite_durations(self):
        result = self.invoke()
        self.assertEqual(1, result.returncode)
        self.assertEqual("", result.stdout)
        self.write([], ["unrelated"])
        self.assertEqual(2, self.invoke().returncode)
        for duration in ("not-a-number", "nan", "inf", "-1"):
            self.write([["id", "tools/build.py", "start", "end", duration, "success", "0", ""]])
            result = self.invoke()
            self.assertEqual(2, result.returncode)
            self.assertIn("CSV line 2", result.stderr)
            self.assertEqual("", result.stdout)


if __name__ == "__main__":
    run(unittest.main)
