"""Verify PostgreSQL initialization through an isolated real binary and API."""
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
    parser.add_argument("--root-only", action="store_true", help="Run only project-root browser checks in the disposable fixture")
    parser.add_argument("--harness-monitor-only", action="store_true", help="Run isolated external harness monitor browser checks")
    parser.add_argument("--stop-feedback-only", action="store_true", help="Verify stop confirmation feedback in the browser")
    args = parser.parse_args()
    if (args.board_only or args.root_only or args.harness_monitor_only or args.stop_feedback_only) and not args.browser:
        parser.error("browser-only selections require --browser")
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
            return json.loads(tk(operation, "--data", json.dumps(data or {}), "-json"))

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
            if args.stop_feedback_only:
                assert "Initialization complete" in tk("init")
                start()
                browser("open", endpoint)
                browser("fill", 'input[name="username"]', "admin")
                browser("fill", 'input[name="password"]', "password")
                browser("click", '#login button[type="submit"],#login button')
                browser("wait", "#welcome-status")
                browser("press", "Escape")
                browser("eval", "window.__originalApi=api;api=(op,data)=>op==='stopSession'?new Promise((resolve,reject)=>{window.__rejectStop=()=>reject(Object.assign(new Error('Stop is recorded, but ownership is unknown and no signal was sent.'),{code:'session_ownership_unknown'}));}):window.__originalApi(op,data);confirmTerminalAction('H3','Stop harness','Confirm the stop request')")
                browser("wait", "#terminal-action-status")
                browser("click", '#editor button[type="submit"]')
                browser("wait", "--fn", "!document.querySelector('#editor').open&&typeof window.__rejectStop==='function'")
                browser("eval", "window.__rejectStop()")
                browser("wait", "--fn", "document.querySelector('#notice').textContent.includes('Stop is recorded')")
                browser("screenshot", "/tmp/teleport-stop-feedback.png")
                print("PASS: stop confirmation closes before the ownership message appears", flush=True)
                return
            if args.harness_monitor_only:
                native = folder / "codex"
                subprocess.run(["go", "build", "-o", str(native), "./tools/fixtures/native-codex"], cwd=ROOT, check=True, timeout=30)
                fixture_env = dict(environment, TELEPORT_TOOL_HISTORY=str(folder / "fixture-history.csv"))
                external = subprocess.Popen([str(native), "app-server"], env=fixture_env, stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                forced = subprocess.Popen([str(native), "app-server"], env=fixture_env, stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                try:
                    assert "Initialization complete" in tk("init")
                    start()
                    browser("open", endpoint)
                    browser("fill", 'input[name="username"]', "admin")
                    browser("fill", 'input[name="password"]', "password")
                    browser("click", '#login button[type="submit"],#login button')
                    browser("wait", "#welcome-status")
                    browser("press", "Escape")
                    browser("click", "#harness-console")
                    browser("wait", f'[data-host-inspect="{external.pid}"]')
                    browser("scrollintoview", f'[data-host-inspect="{external.pid}"]')
                    browser("screenshot", "/tmp/teleport-harness-monitor.png")
                    browser("click", f'[data-host-inspect="{external.pid}"]')
                    browser("wait", "--fn", "document.querySelector('#editor').innerText.includes('Non-Teleport process')")
                    browser("click", '#editor button[type="submit"]')
                    browser("click", "#harness-console")
                    browser("wait", f'[data-host-stop="{external.pid}"]:not([data-force])')
                    browser("scrollintoview", f'[data-host-stop="{external.pid}"]:not([data-force])')
                    browser("click", f'[data-host-stop="{external.pid}"]:not([data-force])')
                    browser("wait", "--fn", "document.querySelector('#editor').innerText.includes('not associated with a Teleport session')")
                    browser("press", "Escape")
                    assert external.poll() is None, "Dismissal stopped the external process"
                    browser("click", "#harness-console")
                    browser("wait", f'[data-host-stop="{external.pid}"]:not([data-force])')
                    browser("scrollintoview", f'[data-host-stop="{external.pid}"]:not([data-force])')
                    browser("click", f'[data-host-stop="{external.pid}"]:not([data-force])')
                    browser("click", '#editor button[type="submit"]')
                    external.wait(timeout=5)
                    assert external.returncode != 0, "The fixture process was not stopped"
                    browser("wait", "--fn", "!document.querySelector('#editor').open")
                    browser("click", "#harness-console")
                    browser("wait", f'[data-host-stop="{forced.pid}"][data-force]')
                    browser("scrollintoview", f'[data-host-stop="{forced.pid}"][data-force]')
                    browser("click", f'[data-host-stop="{forced.pid}"][data-force]')
                    browser("wait", "--fn", "document.querySelector('#editor').innerText.includes('not associated with a Teleport session')")
                    browser("click", '#editor button[type="submit"]')
                    forced.wait(timeout=5)
                    assert forced.returncode != 0, "The force-stop fixture process was not stopped"
                    print("PASS: browser inspects, stops and force-stops owned non-Teleport fixtures; dismissal sends no signal", flush=True)
                    return
                finally:
                    for child in (external, forced):
                        if child.poll() is None:
                            child.terminate()
                            child.wait(timeout=5)
            if args.root_only:
                assert "Initialization complete" in tk("init")
                start()
                browser("open", endpoint)
                browser("fill", 'input[name="username"]', "admin")
                browser("fill", 'input[name="password"]', "password")
                browser("click", '#login button[type="submit"],#login button')
                browser("wait", "#welcome-status")
                browser("click", '#editor button[type="submit"]')
                browser("wait", '#editor [name="projectRoot"]')
                assert json.loads(browser("eval", "document.querySelector('#editor [name=projectRoot]').value")) == "~/code/teleport"
                browser("click", "#cancel")
                browser("select", "#project", "new-project")
                browser("wait", '#editor [name="root"]')
                assert json.loads(browser("eval", "document.querySelector('#editor [name=root]').value")) == "~/code/teleport"
                browser("fill", '#editor [name="name"]', "Root fixture")
                browser("fill", '#editor [name="root"]', "~/code/fixture")
                browser("click", '#editor button[type="submit"]')
                browser("wait", "--fn", "projects.some(p=>p.name==='Root fixture')")
                project = api("listProjects")["items"][0]
                assert project["root"] == "~/code/fixture" and not project.get("repository")
                browser("click", "#account")
                browser("click", "#account-project-settings")
                browser("wait", "#configure-repository")
                browser("click", "#configure-repository")
                browser("wait", '#editor [name="repository"]')
                assert json.loads(browser("eval", "document.querySelector('#editor [name=repository]').value")) == "~/code/fixture"
                browser("eval", "if(document.querySelector('#editor [name=storageRoot]'))throw new Error('Worktree setting remains')")
                browser("screenshot", "/tmp/teleport-project-root.png")
                print("PASS: browser project root defaults, creation, and worktree-free project settings", flush=True)
                return
            assert "Initialization complete" in tk("init")
            first = start()
            assert api("listProjects")["items"] == []
            project = json.loads(tk("setup", "-json", input="Fixture project\n2\nkeep\n"))
            assert project["name"] == "Teleport" and project["maxAgents"] == 2
            assert "already exists" in tk("setup", fail=True)
            api("createTicket", {"projectId": project["id"], "title": "Fixture human task", "agent": False, "requestId": "init-task"})
            assert not first["settings"]["agents"]
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
            tk("init", fail=True)
            assert api("getSession")["installationId"] == first["installationId"]
            api("saveProject", {"id": project["id"], "expectedVersion": project["version"], "description": "Human-edited sample"})
            disposable = api("createProject", {"name": "Disposable", "requestId": "disposable-project"})
            api("createTicket", {"projectId": disposable["id"], "title": "Temporary work", "requestId": "disposable-ticket"})
            api("saveIdea", {"projectId": disposable["id"], "title": "Temporary idea", "text": "test", "requestId": "disposable-idea"})
            tk("project", "rm", "Disposable", fail=True, input="no\n")
            assert len(api("listProjects")["items"]) == 2
            tk("project", "rm", "Disposable", input="Disposable\n")
            assert [p["id"] for p in api("listProjects")["items"]] == [project["id"]]
            stop()
            tk("init")
            again = start()
            assert again["installationId"] == first["installationId"]
            assert api("listProjects")["items"][0]["description"] == "Human-edited sample"
            stop()
            assert "flag provided but not defined" in tk("init", "-f", fail=True)
            assert "flag provided but not defined" in tk("init", "-demo", fail=True)
            print("PASS: PostgreSQL bootstrap, idempotent human edits, API hierarchy, owned-init refusal and removed reset refusal", flush=True)
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
