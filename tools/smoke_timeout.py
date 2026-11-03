"""Supervise smoke workers with a 30-second budget, including owned cleanup."""
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time

from tool_history import ToolTimeout, current_invocation_id

TIMEOUT_SECONDS = 30
WORKER = "TELEPORT_SMOKE_WORKER"
DIRECTORY = "TELEPORT_SMOKE_DIRECTORY"


def worker_directory(script):
    marker = os.environ.pop(WORKER, None)
    if marker == str(Path(script).resolve()):
        return Path(os.environ.pop(DIRECTORY))
    return None


def execute_worker(action, folder):
    try:
        action(folder)
    except BaseException as error:
        from smoke_profile import record_interruption
        record_interruption(folder, "failure")
        (folder / "smoke-result.json").write_text(json.dumps({"error_type": type(error).__name__}))
        raise


def remaining(deadline, maximum):
    return max(0.001, min(maximum, deadline - time.monotonic()))


def stop_group(process, deadline):
    if os.name == "nt":
        if process.poll() is None:
            subprocess.run(["taskkill", "/T", "/F", "/PID", str(process.pid)],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                           timeout=remaining(deadline, 1))
        return
    # start_new_session gives this worker an owned process group, including tk
    # serve and its command children. Never signal the caller's process group.
    for signum in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(process.pid, signum)
        except ProcessLookupError:
            break
        try:
            process.wait(timeout=remaining(deadline, 0.3))
        except subprocess.TimeoutExpired:
            continue
    if process.poll() is None:
        process.wait(timeout=remaining(deadline, 0.5))


def close_browser(folder, deadline):
    control = folder / "smoke-control.json"
    if not control.exists():
        return
    try:
        name = json.loads(control.read_text())["browserSession"]
        if not re.fullmatch(r"teleport-smoke-\d+", name):
            raise ValueError("invalid owned browser session")
        subprocess.run(["agent-browser", "--session", name, "close"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                       check=True, timeout=remaining(deadline, 3))
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print(f"smoke cleanup: browser close was not confirmed ({type(error).__name__})", file=sys.stderr)


def supervise(script, timeout_seconds=TIMEOUT_SECONDS, python_executable=None, provision_database=False):
    # Provisioning and shutdown stay inside the existing total budget.
    started = time.monotonic()
    from contextlib import nullcontext
    from postgres import cluster
    context = cluster(deadline=started+timeout_seconds,work_deadline=started+timeout_seconds-min(6,timeout_seconds/2)) if provision_database and "--database" not in sys.argv else nullcontext(dict(os.environ))
    try:
        with context as environment:
            return _supervise(script, timeout_seconds, python_executable, started, environment)
    except subprocess.TimeoutExpired:
        raise ToolTimeout(124)


def _supervise(script, timeout_seconds, python_executable, started, cluster_environment):
    deadline = started + timeout_seconds
    # Stop test work early enough to keep cleanup inside the full budget.
    reserve = min(6, timeout_seconds / 2)
    timed_out, code = False, None
    with tempfile.TemporaryDirectory(prefix="teleport-smoke-") as temporary:
        folder = Path(temporary)
        environment = dict(cluster_environment, **{WORKER: str(Path(script).resolve()), DIRECTORY: temporary, "TELEPORT_TOOL_RUN_ID": current_invocation_id()})
        options = {"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if os.name == "nt" else {"start_new_session": True}
        process = subprocess.Popen([python_executable or sys.executable, "-B", str(script), *sys.argv[1:]], env=environment, **options)
        try:
            code = process.wait(timeout=max(0.001, deadline - reserve - time.monotonic()))
        except subprocess.TimeoutExpired:
            timed_out = True
        finally:
            try:
                try:
                    stop_group(process, deadline)
                except (OSError, subprocess.SubprocessError) as error:
                    print(f"smoke cleanup: owned process stop was not confirmed ({type(error).__name__})", file=sys.stderr)
            finally:
                if timed_out or code != 0:
                    from concurrent.futures import ThreadPoolExecutor
                    with ThreadPoolExecutor(max_workers=3) as cleanup:
                        list(cleanup.map(lambda owned: close_browser(owned, deadline), [folder, *[path.parent for path in folder.glob("*/smoke-control.json")]]))
        if timed_out:
            from smoke_profile import record_timeout
            record_timeout(folder)
            for progress in folder.glob("*/smoke-progress.json"):
                record_timeout(progress.parent)
            print(f"Smoke timed out: {timeout_seconds:g}s total budget, including cleanup", file=sys.stderr)
            raise ToolTimeout(124)
        if code:
            failure = SystemExit(code if code > 0 else 128 - code)
            result = folder / "smoke-result.json"
            if result.exists():
                failure.error_type = json.loads(result.read_text()).get("error_type", "SystemExit")
            raise failure
