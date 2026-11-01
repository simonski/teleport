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
    parser.add_argument("--database", help="Empty disposable database owned by the parity runner")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="teleport-smoke-") as temporary:
        folder = Path(temporary)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        endpoint = f"http://127.0.0.1:{port}"
        environment = dict(os.environ, TELEPORT_DB=args.database or str(folder / "database.db"), TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
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
                    result = subprocess.run(["agent-browser", "--session", f"teleport-smoke-{port}", *arguments], text=True, capture_output=True, timeout=40)
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
                    agent_ticket = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Demonstrate supervised delivery", "requestId": "agent-ticket"})))
                    configuration = json.loads(tk("getSession"))["settings"]
                    tk("setExecutionMode", "--data", json.dumps({"mode": "demo", "expectedVersion": configuration["version"]}))
                    tk("requestRefinement", "--data", json.dumps({"id": agent_ticket["id"], "expectedVersion": agent_ticket["version"]}))
                    def wait_ticket(predicate):
                        deadline = time.monotonic() + 12
                        while time.monotonic() < deadline:
                            current = json.loads(tk("getTicket", "--data", json.dumps({"id": agent_ticket["id"]})))
                            if predicate(current):
                                return current
                            time.sleep(0.1)
                        raise AssertionError("agent ticket did not reach expected state")
                    wait_ticket(lambda current: current.get("refinement") is not None)
                    browser("find", "role", "button", "click", "--name", "Board", "--exact")
                    browser("wait", "--fn", "document.body.innerText.includes('Demonstrate supervised delivery')")
                    browser("find", "role", "button", "click", "--name", "Demonstrate supervised delivery")
                    browser("wait", '[data-agent="accept"]')
                    browser("click", '[data-agent="accept"]')
                    browser("find", "label", "Accept this exact scope and basis", "check")
                    browser("find", "role", "button", "click", "--name", "Accept scope", "--exact")
                    browser("wait", "--fn", "!document.querySelector('dialog[open]')")
                    candidate = wait_ticket(lambda current: current["status"] == "Approval")
                    assert candidate["attempts"] == 3 and candidate["agentReview"]["simulated"]
                    tk("approveTicket", "--data", json.dumps({"id": candidate["id"], "expectedVersion": candidate["version"], "confirm": True}), success=False)
                    configuration = json.loads(tk("getSession"))["settings"]
                    tk("setExecutionMode", "--data", json.dumps({"mode": "off", "expectedVersion": configuration["version"]}))
                    repository = folder / "repository"
                    repository.mkdir()
                    git_environment = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull, GIT_AUTHOR_NAME="Smoke", GIT_AUTHOR_EMAIL="smoke@localhost", GIT_COMMITTER_NAME="Smoke", GIT_COMMITTER_EMAIL="smoke@localhost")
                    def git(path, *arguments):
                        subprocess.run(["git", "-c", f"core.hooksPath={os.devnull}", "-c", "commit.gpgsign=false", "-C", str(path), *arguments], env=git_environment, check=True, capture_output=True, timeout=30)
                    git(repository, "init", "-b", "main")
                    (repository / "README.md").write_text("Original fixture\n")
                    git(repository, "add", ".")
                    git(repository, "commit", "-m", "Initial")
                    project = next(item for item in json.loads(tk("listProjects"))["items"] if item["id"] == project["id"])
                    external = json.loads(tk("copyWorkflow", "--data", json.dumps({"sourceKey": "agent@1", "projectId": project["id"], "title": "Explicit external delivery", "requestId": "external-flow"})))
                    external["phases"] = [phase for phase in external["phases"] if phase["role"] != "Review"]
                    external["rules"] = [rule for rule in external["rules"] if rule["source"] != "Review"]
                    for rule in external["rules"]:
                        if rule.get("target") == "Review":
                            rule["target"] = "Approval"
                    external = json.loads(tk("saveWorkflow", "--data", json.dumps({"definition": external, "expectedRevision": external["draftRevision"]})))
                    project = json.loads(tk("activateWorkflowVersion", "--data", json.dumps({"workflowKey": external["id"] + "@draft", "expectedRevision": external["draftRevision"], "projectId": project["id"], "expectedProjectVersion": project["version"]})))
                    project = json.loads(tk("configureLocalRepository", "--data", json.dumps({"projectId": project["id"], "expectedVersion": project["version"], "repository": str(repository), "storageRoot": str(folder / "worktrees")})))
                    real = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Real external fixture", "requestId": "real-ticket"})))
                    configuration = json.loads(tk("getSession"))["settings"]
                    tk("setExecutionMode", "--data", json.dumps({"mode": "demo", "expectedVersion": configuration["version"]}))
                    tk("requestRefinement", "--data", json.dumps({"id": real["id"], "expectedVersion": real["version"]}))
                    agent_ticket = real
                    refined = wait_ticket(lambda current: current.get("refinement") is not None)
                    configuration = json.loads(tk("getSession"))["settings"]
                    tk("setExecutionMode", "--data", json.dumps({"mode": "off", "expectedVersion": configuration["version"]}))
                    tk("acceptRefinement", "--data", json.dumps({"id": real["id"], "expectedVersion": refined["version"], "proposalId": refined["refinement"]["id"]}))
                    browser("wait", "--fn", "document.body.innerText.includes('Real external fixture')")
                    browser("find", "role", "button", "click", "--name", "Real external fixture")
                    browser("wait", '[data-delivery="prepare"]')
                    browser("click", '[data-delivery="prepare"]')
                    browser("find", "role", "button", "click", "--name", "Prepare worktree", "--exact")
                    browser("wait", "--fn", "!document.querySelector('dialog[open]')")
                    prepared = wait_ticket(lambda current: current.get("delivery", {}).get("worktree", {}).get("path"))
                    worktree = Path(prepared["delivery"]["worktree"]["path"])
                    (worktree / "README.md").write_text("Delivered fixture\n")
                    git(worktree, "add", ".")
                    git(worktree, "commit", "-m", "Deliver external fixture")
                    browser("find", "role", "button", "click", "--name", "Real external fixture")
                    browser("wait", '[data-delivery="report"]')
                    browser("click", '[data-delivery="report"]')
                    browser("find", "role", "button", "click", "--name", "Capture candidate", "--exact")
                    browser("wait", "--fn", "!document.querySelector('dialog[open]')")
                    browser("find", "role", "button", "click", "--name", "Real external fixture")
                    browser("wait", '[data-delivery="approve"]')
                    browser("click", '[data-delivery="approve"]')
                    browser("find", "label", "Approve this exact code and completion basis", "check")
                    browser("find", "role", "button", "click", "--name", "Approve and fast-forward main", "--exact")
                    browser("wait", "--fn", "!document.querySelector('dialog[open]')")
                    done = wait_ticket(lambda current: current["status"] == "Done")
                    assert done.get("agentReview") is None
                    assert (repository / "README.md").read_text() == "Delivered fixture\n"
                    browser("screenshot", "/tmp/teleport-delivery.png")
                    browser("find", "role", "button", "click", "--name", "Ideas", "--exact")
                    browser("click", "#new-idea")
                    browser("find", "label", "Title", "fill", "A calmer boarding process")
                    browser("find", "label", "Idea", "fill", "Keep the initial thought while creating a separate plan.", "--exact")
                    browser("find", "role", "button", "click", "--name", "Create idea", "--exact")
                    browser("wait", "#idea-propose")
                    browser("click", "#idea-propose")
                    browser("fill", '[name="title"]', "Prepare the boarding checklist")
                    browser("select", '[name="intent"]', "human")
                    browser("find", "role", "button", "click", "--name", "Save proposal", "--exact")
                    browser("wait", "[data-accept-proposal]")
                    browser("click", "[data-accept-proposal]")
                    browser("find", "label", "Create or link this exact proposed work", "check")
                    browser("find", "role", "button", "click", "--name", "Accept proposal", "--exact")
                    browser("wait", ".accepted-label")
                    browser("screenshot", "/tmp/teleport-ideas.png")
                    source_idea = json.loads(tk("listIdeas", "--data", json.dumps({"projectId": project["id"]})))["items"][0]
                    assert source_idea["title"] == "A calmer boarding process" and source_idea["acceptedCount"] == 1
                    targets = json.loads(tk("getIdeaLinks", "--data", json.dumps({"id": source_idea["id"]})))["items"]
                    idea_ticket = json.loads(tk("getTicket", "--data", json.dumps({"id": targets[0]["ticketId"]})))
                    assert idea_ticket["title"] == "Prepare the boarding checklist" and idea_ticket["status"] == "Todo"
                    browser("click", "#dismiss")
                    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
                    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('entities')")
                    browser("click", f'[data-node="ticket:{idea_ticket["id"]}"]')
                    browser("fill", '[data-coordinate="x"]', "125")
                    browser("find", "role", "button", "click", "--name", "Apply position", "--exact")
                    browser("click", "#graph-save")
                    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('saved')")
                    graph = json.loads(tk("getGraph", "--data", json.dumps({"projectId": project["id"]})))
                    assert graph["layout"]["positions"][f'ticket:{idea_ticket["id"]}']["x"] == 125
                    assert json.loads(tk("getTicket", "--data", json.dumps({"id": idea_ticket["id"]})))["version"] == idea_ticket["version"]
                    browser("eval", "window.__graphCamera=JSON.stringify(graphRuntime.scene.snapshot())")
                    changed_ticket = json.loads(tk("updateTicket", "--data", json.dumps({"id": idea_ticket["id"], "expectedVersion": idea_ticket["version"], "priority": "high"})))
                    browser("wait", "--fn", f'tickets.some(t=>t.id==={idea_ticket["id"]}&&t.version==={changed_ticket["version"]})&&!!graphRuntime')
                    browser("eval", "if(JSON.stringify(graphRuntime.scene.snapshot())!==window.__graphCamera)throw new Error('Graph update changed camera or selection')")
                    browser("eval", "window.scrollTo(0,0)")
                    browser("screenshot", "/tmp/teleport-graph.png")
                    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
                    browser("select", "#appearance-style", "instrument")
                    browser("select", "#appearance-mode", "dark")
                    browser("click", "#save-appearance")
                    browser("wait", "--fn", "document.documentElement.dataset.mode==='dark'")
                    prefs = json.loads(tk("getViewPreferences"))
                    assert prefs["style"] == "instrument" and prefs["mode"] == "dark"
                    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
                    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('entities')")
                    browser("screenshot", "/tmp/teleport-graph-dark.png")
                    browser("set", "viewport", "390", "844")
                    browser("screenshot", "/tmp/teleport-mobile.png")
                    browser("set", "viewport", "1280", "800")
                    browser("find", "role", "button", "click", "--name", "Cost", "--exact")
                    browser("eval", "window.__nativeContext=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(kind,...args){return kind.startsWith('webgl')?null:window.__nativeContext.call(this,kind,...args)}")
                    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
                    browser("wait", "--fn", "document.querySelector('#graph-stage')?.innerText.includes('WebGL is unavailable')")
                    browser("click", f'[data-node="ticket:{idea_ticket["id"]}"]')
                    browser("wait", "#graph-apply-position")
                    browser("eval", "HTMLCanvasElement.prototype.getContext=window.__nativeContext")
                    browser("screenshot", "/tmp/teleport-graph-fallback.png")
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
