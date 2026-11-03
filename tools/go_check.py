"""Run Go tests; explicit race runs use low scheduling priority and bounded parallelism."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]


def test_command(args, environment):
    environment = dict(environment)
    command = ["go", "test"]
    if args.race:
        environment.setdefault("GOMAXPROCS", "2")
        command += ["-race", "-p", "1"]
    if args.run:
        command += ["-run", args.run]
    if args.count is not None:
        command += ["-count", str(args.count)]
    if args.profile:
        destination = args.profile.resolve()
        destination.parent.mkdir(parents=True, exist_ok=True)
        command += ["-count=1", "-cpuprofile", str(destination), "-o", str(destination.with_suffix(".test"))]
    command += args.packages or ["./..."]
    if args.race and os.name == "posix":
        nice = shutil.which("nice")
        if not nice:
            raise RuntimeError("Race checks require nice to lower process scheduling priority")
        command = [nice, "-n", "15", *command]
    return command, environment


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--race", action="store_true")
    parser.add_argument("--run", help="Go test name pattern")
    parser.add_argument("--count", type=int, help="Explicit repeat count; omit to retain Go's test cache")
    parser.add_argument("--profile", type=Path, help="CPU profile for exactly one package; also saves its test binary")
    parser.add_argument("packages", nargs="*")
    args = parser.parse_args()
    if args.count is not None and args.count < 1:
        parser.error("--count must be positive")
    if args.profile and (len(args.packages) != 1 or "..." in args.packages[0]):
        parser.error("--profile requires exactly one package without ...")
    command, environment = test_command(args, os.environ)
    options = {}
    if args.race and os.name == "nt":
        options["creationflags"] = subprocess.IDLE_PRIORITY_CLASS
    mode = "race" if args.race else "normal"
    if args.race:
        print(f"Go race tests: low scheduling priority, GOMAXPROCS={environment['GOMAXPROCS']}, one package at a time", flush=True)
    started = time.monotonic()
    result = subprocess.run(command, cwd=ROOT, env=environment, **options)
    code = result.returncode if result.returncode >= 0 else 128 - result.returncode
    print(f"Go {mode} tests: {'PASS' if code == 0 else 'FAIL'} ({time.monotonic() - started:.3f}s)", flush=True)
    if code:
        raise SystemExit(code)


if __name__ == "__main__":
    from tool_history import run
    run(main)
