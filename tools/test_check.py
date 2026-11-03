"""Verify lightweight check boundaries, complete opt-in coverage and resource limits without running suites."""
import os
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import check
import web_check
from check_resources import check_environment, scheduled


class CheckTest(unittest.TestCase):
    def setUp(self):
        quiet = patch("sys.stdout", new=io.StringIO())
        quiet.start()
        self.addCleanup(quiet.stop)

    def test_quick_profile_excludes_expensive_processes_and_keeps_full_gate_coverage(self):
        quick = check.steps("quick")
        integration = check.steps("integration")
        quick_commands = " ".join(" ".join(command or []) for _, command in quick)
        for excluded in ("go_check.py", "go vet", "test_tool_history", "test_smoke_timeout", "test_browser_driver"):
            self.assertNotIn(excluded, quick_commands)
        names = {part.rsplit("/", 1)[-1].removesuffix(".py") for _, command in quick + integration for part in command or []}
        self.assertTrue(set(check.FAST_TESTS) <= names)
        self.assertTrue({"test_tool_history", "test_smoke_timeout", "test_browser_driver", "go_check", "spec_sources", "client_scope", "web_check"} <= names)
        vet = next(command for _, command in integration if command[:2] == ["go", "vet"])
        self.assertEqual(vet, ["go", "vet", "-p", "1", "./..."])

    def test_profile_fails_at_first_error_and_preserves_exit_status(self):
        with patch("check.steps", return_value=[("one", ["fixture"]), ("must not run", ["other"])]), patch("check.subprocess.run", return_value=subprocess.CompletedProcess([], 7)) as run:
            with self.assertRaises(SystemExit) as failure:
                check.execute("quick", {})
        self.assertEqual(failure.exception.code, 7)
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.kwargs["env"]["GOMAXPROCS"], "2")

    def test_unformatted_source_fails_without_modifying_it(self):
        with patch("check.steps", return_value=[("format", ["gofmt", "-l", "fixture.go"])]), patch("check.subprocess.run", return_value=subprocess.CompletedProcess([], 0, "fixture.go\n", "")):
            with self.assertRaises(SystemExit):
                check.execute("quick", {})

    def test_python_syntax_is_read_only_and_does_not_execute_tools(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "tools").mkdir()
            (root / "tools/unsafe.py").write_text("raise RuntimeError('must never execute')")
            check.python_syntax(root)
            (root / "tools/invalid.py").write_text("def broken(")
            with self.assertRaises(SyntaxError):
                check.python_syntax(root)
            self.assertFalse((root / "tools/__pycache__").exists())

    def test_cpu_override_is_preserved_and_node_tests_are_serial(self):
        original = {"GOMAXPROCS": "1", "OTHER": "kept"}
        self.assertEqual(check_environment(original), original)
        self.assertEqual(check_environment({})["GOMAXPROCS"], "2")
        with patch("web_check.subprocess.run") as run, patch("web_check.shutil.which", return_value="fixture-node"):
            web_check.main()
        node_tests = next(call.args[0] for call in run.call_args_list if "--test" in call.args[0])
        self.assertIn("--test-concurrency=1", node_tests)
        if os.name == "posix":
            with patch("check_resources.shutil.which", return_value="/usr/bin/nice"):
                self.assertEqual(scheduled(["fixture"]), ["/usr/bin/nice", "-n", "15", "fixture"])

    def test_python_unit_modules_share_one_interpreter_and_resolve_in_tools(self):
        unit = next(step for step in check.steps("quick") if step[1] and "unittest" in step[1])
        for name in check.FAST_TESTS:
            self.assertTrue((check.ROOT / "tools" / (name + ".py")).is_file(), name)
        with patch("check.steps", return_value=[unit]), patch("check.subprocess.run", return_value=subprocess.CompletedProcess([], 0)) as run:
            check.execute("quick", {})
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.kwargs["cwd"], check.ROOT / "tools")


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
