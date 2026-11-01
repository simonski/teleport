"""Exercise real tool history across success, failures, signals and concurrent writers."""
import ast
import csv
from datetime import datetime
import os
from pathlib import Path
import signal
import shutil
import subprocess
import sys
import tempfile
import unittest

from tool_history import FIELDS, ROOT, run


class ToolHistoryTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="teleport-history-")
        self.addCleanup(self.temporary.cleanup)
        self.path = Path(self.temporary.name) / "history.csv"
        self.environment = dict(os.environ, TELEPORT_TOOL_HISTORY=str(self.path))

    def invoke(self, mode):
        return subprocess.run([sys.executable, "-B", __file__, "--history-fixture", mode],
                              env=self.environment, capture_output=True, text=True, timeout=10)

    def rows(self):
        with self.path.open(newline="") as history:
            reader = csv.DictReader(history)
            self.assertEqual(list(FIELDS), reader.fieldnames)
            rows = list(reader)
        for row in rows:
            self.assertEqual(set(FIELDS), set(row))
            self.assertGreaterEqual(float(row["duration_seconds"]), 0)
            self.assertLessEqual(datetime.fromisoformat(row["started_at"]), datetime.fromisoformat(row["finished_at"]))
        return rows

    def test_success_errors_and_help_retain_their_exit_status_and_hide_secrets(self):
        self.assertEqual(0, self.invoke("success").returncode)
        self.assertEqual(1, self.invoke("failure").returncode)
        self.assertEqual(3, self.invoke("exit").returncode)
        result = subprocess.run([sys.executable, "-B", str(ROOT / "tools/sbom.py"), "--help"],
                                env=self.environment, capture_output=True, text=True, timeout=10)
        self.assertEqual(0, result.returncode)
        rows = self.rows()
        self.assertEqual(["success", "failure", "failure", "success"], [r["outcome"] for r in rows])
        self.assertEqual(["0", "1", "3", "0"], [r["exit_code"] for r in rows])
        self.assertEqual("ValueError", rows[1]["error_type"])
        self.assertNotIn("secret-sentinel", self.path.read_text())

    @unittest.skipIf(os.name == "nt", "POSIX termination contract")
    def test_termination_is_recorded_after_cleanup(self):
        process = subprocess.Popen([sys.executable, "-B", __file__, "--history-fixture", "wait"],
                                   env=self.environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            self.assertEqual("ready\n", process.stdout.readline())
            process.send_signal(signal.SIGTERM)
            process.communicate(timeout=10)
            self.assertEqual(143, process.returncode)
            row, = self.rows()
            self.assertEqual("interrupted", row["outcome"])
            self.assertEqual("SIGTERM", row["error_type"])
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate()

    def test_python_and_go_processes_share_one_header_without_losing_rows(self):
        binary = Path(self.temporary.name) / ("native-fixture.exe" if os.name == "nt" else "native-fixture")
        subprocess.run(["go", "build", "-o", str(binary), "./tools/fixtures/native-codex"], cwd=ROOT, check=True, timeout=90)
        commands = [[sys.executable, "-B", __file__, "--history-fixture", "success"], [str(binary), "--version"]]
        processes = [subprocess.Popen(command, env=self.environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                     for _ in range(8) for command in commands]
        for process in processes:
            output, error = process.communicate(timeout=15)
            self.assertEqual(0, process.returncode, (output, error))
        rows = self.rows()
        self.assertEqual(16, len(rows))
        self.assertEqual(16, len({row["invocation_id"] for row in rows}))
        self.assertEqual({"tools/test_tool_history.py", "tools/fixtures/native-codex"}, {row["tool"] for row in rows})
        self.assertTrue(all(row["outcome"] == "success" for row in rows))
        failed = subprocess.run([str(binary)], env=self.environment, capture_output=True, timeout=10)
        self.assertEqual(2, failed.returncode)
        self.assertEqual(("failure", "2"), (self.rows()[-1]["outcome"], self.rows()[-1]["exit_code"]))
        if os.name != "nt":
            process = subprocess.Popen([str(binary), "app-server"], env=self.environment,
                                       stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                process.stdin.write('{"id":1,"method":"initialize"}\n')
                process.stdin.flush()
                self.assertIn('"id":1', process.stdout.readline())
                process.send_signal(signal.SIGTERM)
                process.wait(timeout=10)
                process.communicate(timeout=10)
                self.assertEqual(143, process.returncode)
                self.assertEqual("interrupted", self.rows()[-1]["outcome"])
            finally:
                if process.poll() is None:
                    process.kill()
                    process.communicate()

    @unittest.skipIf(os.name == "nt", "POSIX executable wrapper")
    def test_copied_exec_launcher_records_handoff_without_a_provider_call(self):
        with tempfile.TemporaryDirectory(prefix="teleport-native-acceptance-") as temporary:
            folder = Path(temporary) / "bin"
            folder.mkdir()
            for name in ("native_trust_wrapper.py", "tool_history.py"):
                shutil.copyfile(ROOT / "tools" / name, folder / name)
            (folder / "codex-original").symlink_to(sys.executable)
            result = subprocess.run([sys.executable, str(folder / "native_trust_wrapper.py"), "--version"],
                                    env=self.environment, capture_output=True, text=True, timeout=10)
            self.assertEqual(0, result.returncode, result.stderr)
            row, = self.rows()
            self.assertEqual("tools/native_trust_wrapper.py", row["tool"])
            self.assertEqual("handoff", row["outcome"])
            self.assertEqual("", row["exit_code"])

    def test_every_python_entrypoint_uses_the_recorder(self):
        for path in (ROOT / "tools").glob("*.py"):
            tree = ast.parse(path.read_text())
            entries = [node for node in tree.body if isinstance(node, ast.If) and "__name__" in ast.unparse(node.test)]
            for entry in entries:
                self.assertTrue(any(isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "run"
                                    for node in ast.walk(entry)), path.name)


def fixture():
    mode = sys.argv[-1]
    if mode == "failure":
        raise ValueError("secret-sentinel")
    if mode == "exit":
        raise SystemExit(3)
    if mode == "wait":
        print("ready", flush=True)
        signal.pause()


if __name__ == "__main__":
    run(fixture if "--history-fixture" in sys.argv else unittest.main)
