"""Run parity checks against a disposable, socket-only PostgreSQL installation.

Requires local PostgreSQL 17+ binaries, including initdb, pg_ctl and createdb.
No existing database, configuration or running service is modified.
See https://www.postgresql.org/docs/17/runtime-config-connection.html
"""
import argparse
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
from urllib.parse import urlencode

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-bin", type=Path)
    parser.add_argument("--browser", action="store_true")
    selection = parser.add_mutually_exclusive_group()
    selection.add_argument("--native-only", action="store_true", help="Run only the native fixture against a fresh database")
    selection.add_argument("--scripts-only", action="store_true", help="Run only isolated script jobs against a fresh database")
    selection.add_argument("--terminal-only", action="store_true", help="Run only the plain PTY fixture against a fresh database")
    selection.add_argument("--smoke-only", action="store_true", help="Run only the application smoke against a fresh database")
    selection.add_argument("--shell-only", action="store_true", help="Run shared shell, landing and modal browser checks against a fresh database")
    selection.add_argument("--tests-only", action="store_true", help="Run only Go tests against disposable PostgreSQL")
    parser.add_argument("--test-pattern", help="Go test -run pattern for a focused regression")
    args = parser.parse_args()
    if args.shell_only and not args.browser:
        parser.error("--shell-only requires --browser")
    candidates = [args.postgres_bin]
    if shutil.which("initdb"):
        candidates.append(Path(shutil.which("initdb")).parent)
    candidates.append(Path("/Applications/Postgres.app/Contents/Versions/latest/bin"))
    binary = next((p for p in candidates if p and (p / "initdb").is_file()), None)
    if not binary:
        parser.error("Install PostgreSQL 17+ or supply --postgres-bin; no checks were run")
    environment = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
    with tempfile.TemporaryDirectory(prefix="tkpg-", dir="/tmp") as temporary:
        folder = Path(temporary)
        os.chmod(folder, 0o700)
        data = folder / "data"
        def pg(name, *arguments):
            result = subprocess.run([str(binary / name), *map(str, arguments)], env=environment, capture_output=True, text=True, timeout=60)
            if result.returncode:
                raise RuntimeError(f"PostgreSQL {name} failed: {result.stderr.strip()}")
            return result
        print(pg("postgres", "--version").stdout.strip(), flush=True)
        pg("initdb", "-D", data, "--auth=trust", "--username=teleport", "--encoding=UTF8", "--no-locale")
        options = "-c listen_addresses='' -c unix_socket_permissions=0700 -c unix_socket_directories=" + shlex.quote(temporary)
        started = False
        try:
            pg("pg_ctl", "-D", data, "-l", folder / "server.log", "-o", options, "-w", "start")
            started = True
            connection = "postgresql://teleport@/postgres?" + urlencode({"host": temporary, "sslmode": "disable"})
            checked = dict(environment, TELEPORT_TEST_POSTGRES=connection)
            focused = args.native_only or args.scripts_only or args.terminal_only or args.smoke_only or args.shell_only
            if not focused:
                test_command = ["go", "test", "-race", "-count=1"]
                if args.test_pattern:
                    test_command += ["-run", args.test_pattern]
                subprocess.run([*test_command, "./..."], cwd=ROOT, env=checked, check=True)
            if args.tests_only:
                print("PASS: PostgreSQL Go test selection", flush=True)
                return
            subprocess.run(["make", "build"], cwd=ROOT, env=environment, check=True)
            if not focused or args.smoke_only or args.shell_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "smoke")
                smoke_url = "postgresql://teleport@/smoke?" + urlencode({"host": temporary, "sslmode": "disable"})
                command = ["python3", "-B", "tools/smoke.py", "--database", smoke_url]
                if args.browser:
                    command.append("--browser")
                if args.shell_only:
                    command.append("--shell-only")
                subprocess.run(command, cwd=ROOT, env=environment, check=True)
            if not args.shell_only and not args.smoke_only and not args.native_only and not args.scripts_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "terminal")
                terminal_url = "postgresql://teleport@/terminal?" + urlencode({"host": temporary, "sslmode": "disable"})
                terminal_command = ["python3", "-B", "tools/local_runtime_check.py", "--database", terminal_url]
                if args.browser:
                    terminal_command.append("--browser")
                subprocess.run(terminal_command, cwd=ROOT, env=environment, check=True)
            if not args.shell_only and not args.smoke_only and not args.scripts_only and not args.terminal_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "native_terminal")
                native_url = "postgresql://teleport@/native_terminal?" + urlencode({"host": temporary, "sslmode": "disable"})
                native_command = ["python3", "-B", "tools/local_runtime_check.py", "--database", native_url, "--native-fixture"]
                if args.browser:
                    native_command.append("--browser")
                subprocess.run(native_command, cwd=ROOT, env=environment, check=True)
            if not args.shell_only and not args.smoke_only and not args.native_only and not args.terminal_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "script_runtime")
                script_url = "postgresql://teleport@/script_runtime?" + urlencode({"host": temporary, "sslmode": "disable"})
                script_command = ["python3", "-B", "tools/script_runtime_check.py", "--database", script_url]
                if args.browser:
                    script_command.append("--browser")
                subprocess.run(script_command, cwd=ROOT, env=environment, check=True)
            label = "shared shell and modal browser checks" if args.shell_only else "application smoke" if args.smoke_only else "native protocol/runtime fixture" if args.native_only else "isolated script jobs" if args.scripts_only else "plain PTY fixture" if args.terminal_only else "isolated domain/race suite and real binary/browser smoke with native and script runtimes"
            print("PASS: PostgreSQL " + label, flush=True)
        finally:
            if started:
                pg("pg_ctl", "-D", data, "-m", "fast", "-w", "stop")


if __name__ == "__main__":
    main()
