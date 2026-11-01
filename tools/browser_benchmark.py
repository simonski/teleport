"""Compare identical successful real-browser actions with CLI and persistent drivers."""
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import threading
import time

from browser_driver import Browser
from smoke_profile import Profile


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass


def main(folder):
    (folder / "index.html").write_text('<input id="value"><button id="add">Add</button><output id="result"></output><script src="fixture.js"></script>')
    (folder / "fixture.js").write_text("document.querySelector('#add').onclick=()=>document.querySelector('#result').textContent=document.querySelector('#value').value;")
    server = ThreadingHTTPServer(("127.0.0.1", 0), partial(QuietHandler, directory=str(folder)))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    session = f"teleport-smoke-{server.server_port}"
    (folder / "smoke-control.json").write_text(json.dumps({"browserSession": session}))
    results = {}
    try:
        for mode in ("cli", "persistent"):
            profile = Profile("benchmark/" + mode, folder)
            profile.start("setup")
            def cli(*arguments):
                return subprocess.run(["agent-browser", "--session", session, *arguments],
                                      capture_output=True, text=True, timeout=3, check=True).stdout.strip()
            browser = Browser(session, dict(os.environ)) if mode == "persistent" else cli
            try:
                browser("open", f"http://127.0.0.1:{server.server_port}/")
                profile.start("actions")
                started = time.monotonic()
                for index in range(20):
                    browser("fill", "#value", str(index))
                    browser("click", "#add")
                    assert json.loads(browser("eval", "document.querySelector('#result').textContent")) == str(index)
                results[mode] = time.monotonic() - started
                profile.finish()
            except BaseException:
                profile.finish("failure")
                raise
            finally:
                browser("close")
        print(json.dumps({"iterations": 20, "actions": 60, "action_seconds": results,
                          "speedup": round(results["cli"] / results["persistent"], 2)}))
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=1)


if __name__ == "__main__":
    from browser_setup import browser_python
    from smoke_timeout import execute_worker, supervise, worker_directory
    from tool_history import run
    worker = worker_directory(__file__)
    if worker is not None:
        execute_worker(main, worker)
    else:
        run(lambda: supervise(__file__, python_executable=browser_python()))
