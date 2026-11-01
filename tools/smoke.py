"""Exercise the actual tk init/server/client path in a disposable installation."""
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", action="store_true")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="teleport-smoke-") as temporary:
        folder = Path(temporary)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        endpoint = f"http://127.0.0.1:{port}"
        environment = dict(os.environ, TELEPORT_DB=str(folder / "database.db"), TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
        def tk(*arguments, input=None, success=True):
            result = subprocess.run([str(ROOT / "bin/tk"), *arguments], env=environment, input=input, text=True, capture_output=True, timeout=30)
            if success and result.returncode:
                raise RuntimeError(result.stderr)
            if not success and not result.returncode:
                raise AssertionError("operation unexpectedly succeeded")
            return result.stdout
        assert "Initialization complete" in tk("init")
        process = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}"], env=environment, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 15
            while True:
                try:
                    with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                        assert response.status == 200
                    break
                except OSError:
                    if time.monotonic() > deadline or process.poll() is not None:
                        raise RuntimeError("server failed to start")
                    time.sleep(0.05)
            tk("init", success=False)
            tk("login", "--password-stdin", input="password\n")
            session = json.loads(tk("getSession"))
            assert session["account"]["administrator"] and not session["settings"]["agents"]
            assert len(json.loads(tk("listWorkflows"))["items"]) == 2
            assert len(json.loads(tk("tools"))["items"]) >= 4
            assert "passwordHash" not in json.dumps(session)
            project = json.loads(tk("createProject", "--data", json.dumps({"name": "Smoke flight", "key": "smoke-flight", "requestId": "smoke-project"})))
            ticket = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Pack the flight case", "agent": False, "requestId": "smoke-ticket"})))
            ticket = json.loads(tk("transitionTicket", "--data", json.dumps({"id": ticket["id"], "expectedVersion": ticket["version"], "target": "Doing"})))
            assert ticket["status"] == "Doing"
            if args.browser:
                def browser(*arguments):
                    result = subprocess.run(["agent-browser", "--session", "teleport-smoke", *arguments], text=True, capture_output=True, timeout=40)
                    if result.returncode:
                        raise RuntimeError(result.stderr or result.stdout)
                    return result.stdout
                try:
                    browser("open", endpoint)
                    browser("find", "label", "Username", "fill", "admin")
                    browser("find", "label", "Password", "fill", "password")
                    browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
                    browser("wait", "--fn", "document.body.innerText.includes('Latest work')")
                    browser("find", "role", "button", "click", "--name", "Todos", "--exact")
                    browser("find", "role", "button", "click", "--name", "Pack the flight case")
                    browser("wait", "--fn", "document.querySelector('dialog')?.open")
                    browser("find", "role", "button", "click", "--name", "Move to Done", "--exact")
                    browser("find", "label", "Accept these completion criteria", "check")
                    browser("find", "role", "button", "click", "--name", "Complete ticket", "--exact")
                    browser("wait", "--fn", "!document.querySelector('dialog')?.open")
                    assert json.loads(tk("getTicket", "--data", json.dumps({"id": ticket["id"]})))["status"] == "Done"
                    browser("screenshot", "/tmp/teleport-smoke.png")
                    draft = json.loads(tk("copyWorkflow", "--data", json.dumps({"sourceKey": "human@1", "projectId": project["id"], "title": "Flight workflow", "requestId": "smoke-flow"})))
                    browser("select", "#project", str(project["id"]))
                    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
                    browser("find", "role", "button", "click", "--name", "Workflow editor", "--exact")
                    browser("wait", "#flow-select")
                    browser("select", "#flow-select", draft["id"] + "@draft")
                    browser("fill", '[data-phase="1"] [data-field="title"]', "In flight")
                    browser("click", "#flow-save")
                    browser("wait", "--fn", "document.querySelector('#flow-message')?.innerText.includes('Draft saved')")
                    browser("select", "#preview-source", "Doing")
                    browser("fill", "#preview-outcome", "return")
                    browser("click", "#flow-preview")
                    browser("wait", "--fn", "document.querySelector('#preview-result')?.innerText.includes('Todo')")
                    browser("eval", "document.querySelector('#workflow-editor').scrollTop=0")
                    browser("screenshot", "/tmp/teleport-workflow.png")
                    browser("click", "#flow-activate")
                    browser("click", '#editor button[type="submit"]')
                    browser("wait", "--fn", "!document.querySelector('dialog[open]')")
                    new_ticket = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Fresh workflow pin", "agent": False, "requestId": "new-pin"})))
                    assert new_ticket["workflow"] == draft["id"] + "@1"
                    assert json.loads(tk("getTicket", "--data", json.dumps({"id": ticket["id"]})))["workflow"] == "human@1"
                except Exception:
                    print(browser("snapshot", "-i"))
                    print(browser("errors"))
                    raise
                finally:
                    browser("close")
            tk("logout")
            tk("getSession", success=False)
            print("PASS: real binary init/serve/login/project/ticket/discovery/logout and exclusive storage ownership" + ("; browser Todos completion verified" if args.browser else ""))
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        assert "Initialization complete" in tk("init")

if __name__ == "__main__":
    main()
