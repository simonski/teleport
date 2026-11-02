"""Prove smoke deadlines preserve status, record timeouts and stop owned children."""
import csv
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest

from smoke_timeout import execute_worker, supervise, worker_directory
from tool_history import run


class SmokeTimeoutTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="teleport-smoke-deadline-test-")
        self.addCleanup(self.temporary.cleanup)
        self.folder = Path(self.temporary.name)
        self.history = self.folder / "history.csv"

    def invoke(self, mode):
        started = time.monotonic()
        result = subprocess.run([sys.executable, "-B", __file__, "--deadline-fixture", mode, str(self.folder)],
                                env=dict(os.environ, TELEPORT_TOOL_HISTORY=str(self.history)),
                                text=True, capture_output=True, timeout=14 if mode == "browser-hang" else 8)
        with self.history.open(newline="") as source:
            rows = list(csv.DictReader(source))
        self.assertEqual(1, len(rows), "worker must not create a second invocation row")
        return result, rows[0], time.monotonic() - started

    def test_success_and_failure_keep_the_original_status(self):
        result, row, _ = self.invoke("success")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("success", row["outcome"])
        self.history.unlink()
        result, row, _ = self.invoke("failure")
        self.assertEqual(3, result.returncode)
        self.assertEqual(("failure", "3"), (row["outcome"], row["exit_code"]))

    @unittest.skipIf(os.name == "nt", "POSIX owned-process-group contract")
    def test_timeout_stops_worker_and_descendant_and_is_recorded(self):
        result, row, elapsed = self.invoke("hang")
        self.assertEqual(124, result.returncode, result.stderr)
        self.assertEqual(("timeout", "124", "TimeoutError"), (row["outcome"], row["exit_code"], row["error_type"]))
        self.assertLess(elapsed, 2.5)
        self.assertLess(float(row["duration_seconds"]), 1.5)
        child = int((self.folder / "child.pid").read_text())
        # A killed orphan can briefly remain a zombie until init reaps it.
        status = subprocess.run(["ps", "-o", "stat=", "-p", str(child)], capture_output=True, text=True)
        self.assertTrue(status.returncode != 0 or status.stdout.strip().startswith("Z"), status.stdout)

    @unittest.skipIf(os.name == "nt", "POSIX owned-process-group contract")
    def test_timeout_stops_real_owned_chromium_processes(self):
        from browser_setup import browser_python
        try:
            browser_python()
        except SystemExit:
            self.skipTest("requires make browser-tools")
        result, row, elapsed = self.invoke("browser-hang")
        self.assertEqual((result.returncode, row["outcome"]), (124, "timeout"), result.stderr)
        self.assertLess(elapsed, 13.5)
        for pid in json.loads((self.folder / "browser-pids.json").read_text()):
            status = subprocess.run(["ps", "-o", "stat=", "-p", str(pid)], capture_output=True, text=True)
            self.assertTrue(status.returncode != 0 or status.stdout.strip().startswith("Z"), f"owned browser PID {pid}: {status.stdout}")


def fixture(folder):
    mode, output = sys.argv[-2], Path(sys.argv[-1])
    if mode == "failure":
        raise SystemExit(3)
    if mode == "hang":
        child = subprocess.Popen(["sleep", "60"])
        (output / "child.pid").write_text(str(child.pid))
        time.sleep(60)
    if mode == "browser-hang":
        from browser_driver import Browser
        browser = Browser("teleport-deadline-fixture", dict(os.environ))
        browser("open", "about:blank")
        protocol = browser.connection.new_browser_cdp_session()
        rows = protocol.send("SystemInfo.getProcessInfo")["processInfo"]
        (output / "browser-pids.json").write_text(json.dumps([int(row["id"]) for row in rows]))
        time.sleep(60)


if __name__ == "__main__":
    if "--deadline-fixture" in sys.argv:
        worker = worker_directory(__file__)
        if worker is not None:
            execute_worker(fixture, worker)
        else:
            def launch():
                mode = sys.argv[-2]
                from browser_setup import browser_python
                supervise(__file__, timeout_seconds=12 if mode == "browser-hang" else 1 if mode == "hang" else 3,
                          python_executable=browser_python() if mode == "browser-hang" else None)
            run(launch)
    else:
        run(unittest.main)
