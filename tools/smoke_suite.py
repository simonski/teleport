"""Run isolated acceptance slices inside the supervisor's single owned group."""
import os
import subprocess
import sys
import time

from smoke_profile import Profile, record_interruption
from smoke_timeout import DIRECTORY, WORKER


def run_suite(script, folder):
    processes, logs = [], []
    driver = "cli" if "--browser-driver=cli" in sys.argv or any(sys.argv[i:i+2] == ["--browser-driver", "cli"] for i in range(len(sys.argv))) else "persistent"
    profile = Profile("sqlite/full-suite-" + driver, folder)
    profile.start("suite")
    try:
        for name in ("core", "shell", "views"):
            child = folder / name
            child.mkdir()
            output = (folder / (name + ".log")).open("w+")
            logs.append(output)
            environment = dict(os.environ, **{WORKER: str(script), DIRECTORY: str(child)})
            # Inherit the supervised process group: expiry stops every slice,
            # server and command child together, without nested budget resets.
            processes.append(subprocess.Popen([sys.executable, "-B", str(script), *sys.argv[1:], "--browser-slice", name],
                                              env=environment, stdout=output, stderr=subprocess.STDOUT))
        while any(process.poll() is None for process in processes):
            if any(process.poll() not in (None, 0) for process in processes):
                raise RuntimeError("Browser acceptance slice failed")
            time.sleep(.02)
        if any(process.returncode for process in processes):
            raise RuntimeError("Browser acceptance slice failed")
        profile.finish()
        print("PASS: all browser acceptance slices within the shared 30-second budget", flush=True)
    except BaseException:
        profile.finish("failure")
        raise
    finally:
        for process in processes:
            if process.poll() is None:
                process.terminate()
        for process in processes:
            try:
                process.wait(timeout=.5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=.5)
        for output in logs:
            output.seek(0)
            print(output.read(), end="", flush=True)
            output.close()
        for child in folder.iterdir():
            if child.is_dir():
                record_interruption(child, "cancelled")
