"""Shared CSV invocation history for executable development tools.

Records UTC instants, monotonic duration, outcome and exit status without arguments,
environment values or exception messages. TELEPORT_TOOL_HISTORY selects a different
CSV for isolated tests. Process replacement records an explicit handoff outcome.
"""
import csv
from contextlib import contextmanager
from datetime import datetime, timezone
import os
from pathlib import Path
import signal
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
FIELDS = ("invocation_id", "tool", "started_at", "finished_at", "duration_seconds", "outcome", "exit_code", "error_type")
_current = None


@contextmanager
def locked_history(path):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with Path(str(path) + ".lock").open("a+b") as lock:
        if os.name == "nt":
            import msvcrt
            lock.seek(0)
            msvcrt.locking(lock.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl
            fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            yield
        finally:
            if os.name == "nt":
                lock.seek(0)
                msvcrt.locking(lock.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                fcntl.flock(lock, fcntl.LOCK_UN)


def append_record(path, row):
    path = Path(path)
    with locked_history(path):
        with path.open("a", newline="", encoding="utf-8") as output:
            writer = csv.writer(output)
            if output.tell() == 0:
                writer.writerow(FIELDS)
            writer.writerow(row)
            output.flush()
            os.fsync(output.fileno())


def read_history(path):
    """Read a complete snapshot while concurrent writers hold the same lock."""
    path = Path(path)
    with locked_history(path):
        with path.open(newline="", encoding="utf-8") as source:
            reader = csv.DictReader(source, strict=True)
            return reader.fieldnames or [], list(reader)


class Invocation:
    def __init__(self, tool):
        self.tool = tool
        self.identity = str(uuid.uuid4())
        self.started = datetime.now(timezone.utc)
        self.clock = time.monotonic()
        self.path = Path(os.environ.setdefault("TELEPORT_TOOL_HISTORY", str(ROOT / "tool_calls_history.csv"))).resolve()
        os.environ["TELEPORT_TOOL_HISTORY"] = str(self.path)

    def record(self, outcome, exit_code, error_type=""):
        finished = datetime.now(timezone.utc)
        row = (self.identity, self.tool, self.started.isoformat().replace("+00:00", "Z"),
               finished.isoformat().replace("+00:00", "Z"), f"{time.monotonic() - self.clock:.6f}",
               outcome, exit_code, error_type)
        try:
            append_record(self.path, row)
        except OSError as error:
            print(f"tool history: unable to record {self.tool}: {error}", file=sys.stderr)


class _Terminated(BaseException):
    def __init__(self, signum):
        self.signum = signum


class ToolTimeout(SystemExit):
    """A supervised tool exhausted its wall-clock budget (exit status 124)."""


def record_handoff():
    """Record an exec handoff; the replacement process owns its eventual result."""
    if _current is not None:
        _current.record("handoff", "")


def run(action, tool=None):
    global _current
    if tool is None:
        source = Path(sys.modules["__main__"].__file__).resolve()
        tool = source.relative_to(ROOT).as_posix()
    invocation = Invocation(tool)
    previous, _current = _current, invocation
    outcome, exit_code, error_type = "success", 0, ""
    old_handler = signal.getsignal(signal.SIGTERM)

    def terminated(signum, _frame):
        raise _Terminated(signum)

    signal.signal(signal.SIGTERM, terminated)
    try:
        return action()
    except ToolTimeout:
        outcome, exit_code, error_type = "timeout", 124, "TimeoutError"
        raise
    except SystemExit as error:
        exit_code = error.code if isinstance(error.code, int) else (0 if error.code is None else 1)
        outcome = "success" if exit_code == 0 else "failure"
        error_type = "" if exit_code == 0 else getattr(error, "error_type", "SystemExit")
        raise
    except KeyboardInterrupt:
        outcome, exit_code, error_type = "interrupted", 130, "KeyboardInterrupt"
        raise
    except _Terminated as error:
        outcome, exit_code, error_type = "interrupted", 128 + error.signum, "SIGTERM"
        raise SystemExit(exit_code)
    except BaseException as error:
        outcome, exit_code, error_type = "failure", 1, type(error).__name__
        raise
    finally:
        signal.signal(signal.SIGTERM, old_handler)
        invocation.record(outcome, exit_code, error_type)
        _current = previous
