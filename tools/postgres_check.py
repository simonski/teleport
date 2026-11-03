"""Run parity checks against a disposable, socket-only PostgreSQL installation.

Requires local PostgreSQL 16+ binaries, including initdb, pg_ctl and createdb.
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
    parser.add_argument("--race", action="store_true", help="Opt into low-priority, bounded Go race tests")
    selection = parser.add_mutually_exclusive_group()
    selection.add_argument("--native-only", action="store_true", help="Run only the native fixture against a fresh database")
    selection.add_argument("--workflows-only", action="store_true", help="Run automatic native workflow fixtures against a fresh database")
    selection.add_argument("--scripts-only", action="store_true", help="Run only isolated script jobs against a fresh database")
    selection.add_argument("--terminal-only", action="store_true", help="Run only the plain PTY fixture against a fresh database")
    selection.add_argument("--smoke-only", action="store_true", help="Run only the application smoke against a fresh database")
    selection.add_argument("--shell-only", action="store_true", help="Run shared shell, landing and modal browser checks against a fresh database")
    selection.add_argument("--studio-only", action="store_true", help="Run 2D graph, Studio and workflow Map acceptance against a fresh database")
    selection.add_argument("--overview-only", action="store_true", help="Run Home, Tickets, Attention and workflow assembly acceptance")
    selection.add_argument("--tests-only", action="store_true", help="Run only Go tests against disposable PostgreSQL")
    selection.add_argument("--init-only", action="store_true", help="Verify initialization against a fresh database")
    parser.add_argument("--test-pattern", help="Go test -run pattern for a focused regression")
    parser.add_argument("--native-large-frame", action="store_true", help="Exercise a large frame in the native fixture (requires --native-only)")
    parser.add_argument("--shell-pty", action="store_true", help="Exercise zsh PTYs (requires --terminal-only)")
    parser.add_argument("--codex-pty", action="store_true", help="Exercise direct Codex PTY fixture (requires --terminal-only)")
    parser.add_argument("--harness-pty", choices=["claude", "copilot"], help="Exercise a direct harness PTY fixture (requires --terminal-only)")
    args = parser.parse_args()
    if args.native_large_frame and not args.native_only:
        parser.error("--native-large-frame requires --native-only")
    if args.shell_pty and not args.terminal_only:
        parser.error("--shell-pty requires --terminal-only")
    if args.codex_pty and (not args.terminal_only or args.shell_pty):
        parser.error("--codex-pty requires --terminal-only and excludes --shell-pty")
    if args.harness_pty and (not args.terminal_only or args.shell_pty or args.codex_pty):
        parser.error("--harness-pty requires --terminal-only and excludes other PTY selections")
    if (args.shell_only or args.studio_only) and not args.browser:
        parser.error("--shell-only/--studio-only requires --browser")
    from postgres import binaries
    binary = args.postgres_bin or binaries()
    if not (binary / "initdb").is_file():
        parser.error("Install PostgreSQL 16+ or supply --postgres-bin; no checks were run")
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
            focused = args.native_only or args.workflows_only or args.scripts_only or args.terminal_only or args.smoke_only or args.shell_only or args.studio_only or args.overview_only or args.init_only
            if not focused:
                test_command = ["python3", "-B", "tools/go_check.py", "--count", "1"]
                if args.race:
                    test_command.append("--race")
                if args.test_pattern:
                    test_command += ["--run", args.test_pattern]
                subprocess.run([*test_command, "./..."], cwd=ROOT, env=checked, check=True)
            if args.tests_only:
                print("PASS: PostgreSQL Go test selection", flush=True)
                return
            subprocess.run(["python3", "-B", "tools/build.py", "--reuse"], cwd=ROOT, env=environment, check=True)
            if args.workflows_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "workflows")
                workflow_url = "postgresql://teleport@/workflows?" + urlencode({"host": temporary, "sslmode": "disable"})
                command = ["python3", "-B", "tools/native_workflow_check.py", "--database", workflow_url]
                if args.browser:
                    command.append("--browser")
                subprocess.run(command, cwd=ROOT, env=environment, check=True)
                return
            if args.init_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "bootstrap")
                bootstrap_url = "postgresql://teleport@/bootstrap?" + urlencode({"host": temporary, "sslmode": "disable"})
                bootstrap_command = ["python3", "-B", "tools/init_check.py", "--database", bootstrap_url]
                if args.browser:
                    bootstrap_command.append("--browser")
                subprocess.run(bootstrap_command, cwd=ROOT, env=environment, check=True)
                return
            if not focused or args.smoke_only or args.shell_only or args.studio_only or args.overview_only:
                commands = []
                for part in (("core", "shell", "views") if args.browser and not args.shell_only and not args.studio_only and not args.overview_only else (None,)):
                    database = "smoke_" + part if part else "smoke"
                    pg("createdb", "--host", temporary, "--username", "teleport", database)
                    smoke_url = "postgresql://teleport@/" + database + "?" + urlencode({"host": temporary, "sslmode": "disable"})
                    command = ["python3", "-B", "tools/smoke.py", "--database", smoke_url]
                    if args.browser:
                        command.append("--browser")
                    if args.shell_only:
                        command.append("--shell-only")
                    if args.studio_only:
                        command.append("--studio-only")
                    if args.overview_only:
                        command.append("--overview-only")
                    if part:
                        command.extend(("--browser-slice", part))
                    commands.append(command)
                from concurrent.futures import ThreadPoolExecutor
                with ThreadPoolExecutor(max_workers=3) as checks:
                    list(checks.map(lambda command: subprocess.run(command, cwd=ROOT, env=environment, check=True), commands))
                if args.studio_only:
                    print("PASS: PostgreSQL 2D graph, Studio and workflow Map acceptance",flush=True)
                    return
                if args.overview_only:
                    print("PASS: PostgreSQL Home, Tickets, Attention and workflow assembly",flush=True)
                    return
            if not args.shell_only and not args.smoke_only and not args.native_only and not args.scripts_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "terminal")
                terminal_url = "postgresql://teleport@/terminal?" + urlencode({"host": temporary, "sslmode": "disable"})
                terminal_command = ["python3", "-B", "tools/local_runtime_check.py", "--database", terminal_url]
                if args.shell_pty:
                    terminal_command.append("--shell-pty")
                if args.codex_pty:
                    terminal_command.append("--codex-pty")
                if args.harness_pty:
                    terminal_command.extend(["--harness-pty", args.harness_pty])
                if args.browser:
                    terminal_command.append("--browser")
                subprocess.run(terminal_command, cwd=ROOT, env=environment, check=True)
            if not args.shell_only and not args.smoke_only and not args.scripts_only and not args.terminal_only:
                pg("createdb", "--host", temporary, "--username", "teleport", "native_terminal")
                native_url = "postgresql://teleport@/native_terminal?" + urlencode({"host": temporary, "sslmode": "disable"})
                native_command = ["python3", "-B", "tools/local_runtime_check.py", "--database", native_url, "--native-fixture"]
                if args.native_large_frame:
                    native_command.append("--native-large-frame")
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
            label = "shared shell and modal browser checks" if args.shell_only else "application smoke" if args.smoke_only else "native protocol/runtime fixture" if args.native_only else "isolated script jobs" if args.scripts_only else "plain PTY fixture" if args.terminal_only else "isolated domain suite and real binary/browser smoke with native and script runtimes"
            print("PASS: PostgreSQL " + label, flush=True)
        finally:
            if started:
                pg("pg_ctl", "-D", data, "-m", "fast", "-w", "stop")


if __name__ == "__main__":
    from tool_history import run
    run(main)
