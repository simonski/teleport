"""Shared scheduling limits for explicitly invoked local development checks."""
import os
import shutil
import subprocess


def check_environment(environment):
    result = dict(environment)
    result.setdefault("GOMAXPROCS", "2")
    return result


def scheduled(command):
    if os.name == "posix":
        nice = shutil.which("nice")
        if not nice:
            raise RuntimeError("Development checks require nice to lower scheduling priority")
        return [nice, "-n", "15", *command]
    return list(command)


def process_options():
    return {"creationflags": subprocess.IDLE_PRIORITY_CLASS} if os.name == "nt" else {}
