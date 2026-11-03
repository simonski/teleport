"""Verify PostgreSQL initialization and PetStore through an isolated real binary and API."""
import argparse
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.request
from human_todos_browser import verify_human_todos
from board_controls_browser import verify_board_controls
from location_browser import verify_location

ROOT = Path(__file__).resolve().parents[1]


def main(database_url=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", help="Empty disposable database owned by the parity runner")
    parser.add_argument("--browser", action="store_true")
    parser.add_argument("--board-only", action="store_true", help="Run only Board browser checks in the disposable fixture")
    args = parser.parse_args()
    if args.board_only and not args.browser:
        parser.error("--board-only requires --browser")
    with tempfile.TemporaryDirectory(prefix="teleport-init-proof-") as temporary:
        folder = Path(temporary)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        endpoint = f"http://127.0.0.1:{port}"
        database = args.database or database_url
        environment = dict(os.environ, TELEPORT_DB=database, TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
        process = None
        browser_name = f"teleport-init-{port}"

        def browser(*arguments):
            result = subprocess.run(["agent-browser", "--session", browser_name, *arguments], capture_output=True, text=True, timeout=40)
            if result.returncode:
                raise RuntimeError(result.stderr or result.stdout)
            return result.stdout

        def tk(*arguments, fail=False, input=None):
            result = subprocess.run([str(ROOT / "bin/tk"), *arguments], env=environment, text=True, input=input, capture_output=True, timeout=30)
            if fail:
                assert result.returncode != 0, result.stdout
                return result.stderr
            if result.returncode:
                raise RuntimeError(result.stderr)
            return result.stdout

        def api(operation, data=None):
            return json.loads(tk(operation, "--data", json.dumps(data or {})))

        def start():
            nonlocal process
            process = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}", "--runtime-dir", str(folder / "runtime")], env=environment, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                try:
                    with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                        if response.status == 200:
                            tk("login", "--password-stdin", input="password\n")
                            return api("getSession")
                except OSError:
                    pass
                if process.poll() is not None:
                    raise AssertionError("Isolated server exited")
                time.sleep(0.05)
            raise AssertionError("Isolated server did not start")

        def stop():
            nonlocal process
            if process:
                process.terminate()
                process.wait(timeout=15)
                process = None

        try:
            assert "PetStore simulation ready" in tk("init", "-demo")
            first = start()
            project = api("listProjects")["items"][0]
            assert project["simulation"] and project["demoSeed"] == "petstore-v1"
            tickets = api("listTickets", {"projectId": project["id"], "limit": 200})["items"]
            ideas = api("listIdeas", {"projectId": project["id"], "limit": 200})["items"]
            assert len(tickets) >= 25 and len(ideas) == 4, (len(tickets), len(ideas), project["id"])
            assert not first["settings"]["agents"] and first["preferences"]["defaultProject"] == project["id"]
            assert all(not ticket["agent"] for ticket in tickets if ticket["status"] == "Done")
            if args.browser:
                browser("open", endpoint)
                browser("fill", 'input[name="username"]', "admin")
                browser("fill", 'input[name="password"]', "password")
                browser("click", '#login button[type="submit"],#login button')
                browser("wait", "#nav-views")
                if args.board_only:
                    project = api("setHandbrake", {"projectId": project["id"], "expectedVersion": project["version"], "engaged": True})
                    browser("reload")
                    browser("wait", "#nav-views")
                else:
                    verify_human_todos(browser, api, project)
                verify_board_controls(browser, api, project, interactions_only=args.board_only)
                if not args.board_only:
                    verify_location(browser, api, project)
                browser("close")
                project = api("listProjects")["items"][0]
            tk("init", "-demo", fail=True)
            assert api("getSession")["installationId"] == first["installationId"]
            api("saveProject", {"id": project["id"], "expectedVersion": project["version"], "description": "Human-edited sample"})
            stop()
            tk("init", "-demo")
            again = start()
            assert again["installationId"] == first["installationId"]
            assert api("listProjects")["items"][0]["description"] == "Human-edited sample"
            stop()
            assert "flag provided but not defined" in tk("init", "-f", fail=True)
            print("PASS: PostgreSQL demo bootstrap, idempotent human edits, API hierarchy, owned-init refusal and removed reset refusal", flush=True)
        except Exception:
            if args.browser:
                print(browser("eval", "JSON.stringify({trace:window.__ticketTrace,notice:document.querySelector('#notice')?.textContent,dialog:document.querySelector('#editor')?.open,calendar:typeof enhanceDate,fields:[...document.querySelectorAll('#editor input')].map(input=>({name:input.name,type:input.type,calendar:input.dataset.calendar}))})"), flush=True)
                print(browser("errors"), flush=True)
                browser("screenshot", "/tmp/teleport-init-browser-failure.png")
            raise
        finally:
            if args.browser:
                subprocess.run(["agent-browser", "--session", browser_name, "close"], capture_output=True, timeout=15)
            stop()


if __name__ == "__main__":
    from tool_history import run
    from postgres import run_with_database
    run(lambda: run_with_database(main))
