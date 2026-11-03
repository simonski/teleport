"""Plan or run lightweight checks; expensive integration checks require an explicit profile."""
import argparse
import ast
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from check_resources import check_environment, process_options, scheduled

ROOT = Path(__file__).resolve().parents[1]
FAST_TESTS = (
    "test_build", "test_sbom", "test_timing", "test_smoke_profile", "test_source_cache",
    "test_deploy", "test_go_check", "test_postgres", "test_check", "test_capture_backlog",
)


def steps(profile):
    python = sys.executable
    source = [
        ("Go formatting", ["gofmt", "-l", "cmd", "internal", "tools"]),
        ("Python source syntax", None),
        ("Browser source, serial unit tests and vendor integrity", [python, "-B", "tools/web_check.py"]),
    ]
    vet = ("Go vet (one package at a time)", ["go", "vet", "-p", "1", "./..."])
    if profile == "lint":
        return [*source, vet]
    if profile == "integration":
        return [vet,
            ("Cross-language tool history", [python, "-B", "tools/test_tool_history.py"]),
            ("Owned-process and real-browser timeout cleanup", [python, "-B", "tools/test_smoke_timeout.py"]),
            ("Real browser driver", [python, "-B", "tools/test_browser_driver.py"]),
            ("Go tests with disposable PostgreSQL", [python, "-B", "tools/go_check.py"]),
        ]
    return [*source,
        ("Tool unit tests (one interpreter)", [python, "-B", "-m", "unittest", *FAST_TESTS]),
        ("Specification references", [python, "-B", "tools/spec_sources.py", "--verify"]),
        ("Supported client scope", [python, "-B", "tools/client_scope.py"]),
    ]


def python_syntax(root):
    for path in sorted((root / "tools").rglob("*.py")) + sorted((root / "tool").glob("*.py")):
        ast.parse(path.read_text(), filename=str(path))


def execute(profile, environment=None):
    environment = check_environment(os.environ if environment is None else environment)
    print(f"{profile} checks: low priority, GOMAXPROCS={environment['GOMAXPROCS']}, serial packages/test files", flush=True)
    for name, command in steps(profile):
        started = time.monotonic()
        print(name + "…", flush=True)
        if command is None:
            python_syntax(ROOT)
        else:
            formatting = command[0] == "gofmt"
            cwd = ROOT / "tools" if command[2:4] == ["-m", "unittest"] else ROOT
            result = subprocess.run(scheduled(command), cwd=cwd, env=environment, **process_options(), **({"capture_output": True, "text": True} if formatting else {}))
            if result.returncode:
                if formatting:
                    print(result.stderr, file=sys.stderr, end="")
                raise SystemExit(result.returncode if result.returncode > 0 else 128 - result.returncode)
            if formatting and result.stdout:
                print(result.stdout, end="")
                raise SystemExit("Go formatting differs; run gofmt on the listed files")
        print(f"PASS: {name} ({time.monotonic()-started:.3f}s)", flush=True)
    if profile == "quick":
        print("Go vet/tests, PostgreSQL, real browsers and deadline exercises are explicit: make check-integration; race: make race-check", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--integration", action="store_true")
    group.add_argument("--lint", action="store_true")
    parser.add_argument("--plan", action="store_true", help="Print the exact profile without starting any check")
    args = parser.parse_args()
    profile = "integration" if args.integration else "lint" if args.lint else "quick"
    if args.plan:
        print(json.dumps({"profile": profile, "GOMAXPROCS": check_environment(os.environ)["GOMAXPROCS"], "steps": [{"name": name, "command": command} for name, command in steps(profile)]}, indent=2))
        return
    execute(profile)


if __name__ == "__main__":
    from tool_history import run
    run(main)
