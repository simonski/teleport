"""Prove actual daemon/PTY reconnect with an owned zero-model binary fixture."""
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
import uuid

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", action="store_true")
    parser.add_argument("--native-fixture", action="store_true", help="Exercise native Codex protocol through a zero-provider fake executable")
    parser.add_argument("--database", help="Existing empty disposable PostgreSQL database")
    args = parser.parse_args()
    folder = Path(tempfile.mkdtemp(prefix="teleport-terminal-proof-"))
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    environment = dict(os.environ, TELEPORT_DB=args.database or str(folder / "database.db"), TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
    if args.native_fixture:
        fixture_bin = folder / "fixture-bin"
        fixture_bin.mkdir()
        subprocess.run(["go", "build", "-o", str(fixture_bin / "codex"), "./tools/fixtures/native-codex"], cwd=ROOT, check=True, timeout=60)
        environment["PATH"] = str(fixture_bin) + os.pathsep + environment["PATH"]
    harness = "codex" if args.native_fixture else "fixture"
    process = None
    session_id = None
    clean = False
    browser_name = f"teleport-terminal-{port}"
    browser_open = False

    def tk(operation, data=None):
        command = [str(ROOT / "bin/tk"), operation]
        if data is not None:
            command += ["--data", json.dumps(data)]
        result = subprocess.run(command, env=environment, text=True, capture_output=True, timeout=15)
        if result.returncode:
            raise RuntimeError(result.stderr)
        return json.loads(result.stdout)

    def wait_for(callback):
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            result = callback()
            if result:
                return result
            time.sleep(0.05)
        raise AssertionError("local terminal proof timed out")

    def serve():
        server = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}", "--runtime-dir", str(folder / "runtime"), "--enable-terminal-fixture"], env=environment, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        def ready():
            try:
                with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                    return response.status == 200
            except OSError:
                return False
        wait_for(ready)
        return server

    def browser(*arguments):
        result = subprocess.run(["agent-browser", "--session", browser_name, *arguments], text=True, capture_output=True, timeout=40)
        if result.returncode:
            raise RuntimeError(result.stderr or result.stdout)
        return result.stdout

    try:
        subprocess.run([str(ROOT / "bin/tk"), "init"], env=environment, check=True, capture_output=True, timeout=15)
        process = serve()
        subprocess.run([str(ROOT / "bin/tk"), "login", "--password-stdin"], env=environment, input="password\n", text=True, check=True, capture_output=True, timeout=15)
        repository = folder / "repository"
        repository.mkdir()
        def git(*arguments):
            subprocess.run(["git", "-c", f"core.hooksPath={os.devnull}", "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "-C", str(repository), *arguments], check=True, capture_output=True, timeout=15)
        git("init", "-b", "main")
        (repository / "fixture.txt").write_text("owned terminal proof\n")
        git("add", "fixture.txt")
        git("commit", "-m", "Seed terminal fixture")
        project = tk("createProject", {"name": "Terminal proof", "key": "terminal-" + uuid.uuid4().hex[:12], "requestId": "terminal-project"})
        project = tk("configureLocalRepository", {"projectId": project["id"], "expectedVersion": project["version"], "repository": str(repository), "storageRoot": str(folder / "worktrees")})
        start = {"projectId": project["id"], "harness": harness, "requestId": "terminal-start"}
        row = tk("openConsoleSession", start)
        session_id = row["id"]
        assert row["state"] == "running" and row["ticketId"] == 0
        assert tk("openConsoleSession", start)["id"] == session_id
        import base64
        def read():
            return base64.b64decode(tk("attachSession", {"id": session_id})["data"]).decode(errors="replace")
        wait_for(lambda: ("native fixture connected" if args.native_fixture else "terminal fixture") in read())
        summary = next(item for item in tk("listProjectSummaries")["items"] if item["id"] == project["id"])
        assert summary["agents"] == 1 and summary["agentsScope"] == "own-authorized" and summary["tokens"] is None
        if args.native_fixture:
            wait_for(lambda: tk("getSessionRequests", {"id": session_id})["items"])
            context_env = dict(environment, TELEPORT_CONFIG=str(folder / "runtime" / (session_id + ".client.json")))
            for operation, permitted in [("listProjects", True), ("getSession", False), ("listSessions", False), ("approveTicket", False)]:
                result = subprocess.run([str(ROOT / "bin/tk"), operation, "--data", "{}"], env=context_env, text=True, capture_output=True, timeout=15)
                assert (result.returncode == 0) == permitted, (operation, result.stdout, result.stderr)
        sent = {"id": session_id, "requestId": "terminal-input-1", "data": base64.b64encode(b"alpha\n").decode()}
        receipt = tk("sendSessionInput", sent)
        assert receipt["state"] == "delivered" and tk("sendSessionInput", sent) == receipt
        wait_for(lambda: ("native fixture answer resolved" if args.native_fixture else "received: alpha") in read())
        assert read().count("native fixture answer resolved" if args.native_fixture else "received: alpha") == 1
        if args.native_fixture:
            native = wait_for(lambda: next((r for r in tk("getSessionRequests", {"id": session_id})["items"] if r["state"] == "resolved" and r.get("source")), None))
            assert native["source"] == "native client" and native["answers"]["secret"]["answers"] == ["[redacted]"]
            assert "fixture-secret" not in (folder / "runtime/sessions.json").read_text()
            def completed_turn():
                current = tk("getSessionTurn", {"id": session_id}).get("turn")
                return current if current and current["state"] == "completed" else None
            turn = wait_for(completed_turn)
            assert turn["state"] == "completed" and turn["text"] == "Native fixture completed" and not turn["truncated"]
            assert not tk("getSessionTurn", {"id": session_id}).get("failure")
        tk("resizeSession", {"id": session_id, "cols": 120, "rows": 35})
        before = json.loads((folder / "runtime/sessions.json").read_text())[session_id]
        if args.browser:
            browser("open", endpoint)
            browser_open = True
            browser("find", "label", "Username", "fill", "admin")
            browser("find", "label", "Password", "fill", "password")
            browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
            browser("wait", "#welcome-status")
            browser("press", "Escape")
            browser("press", "`")
            browser("wait", ".xterm-screen")
            for key in "from browser":
                browser("press", "Space" if key == " " else key)
            browser("press", "Enter")
            browser("wait", "--fn", "terminalView?.terminal.buffer.active.getLine(terminalView.terminal.buffer.active.cursorY)?.translateToString(true)!==undefined")
            wait_for(lambda: read().count("Fixture direction?") >= 2 if args.native_fixture else "received: from browser" in read())
            browser("wait", "--fn", "terminalView&&!terminalView.sending&&terminalView.queue.length===0")
            browser("eval", "window.navigationTerminal=terminalView;true")
            preferences = tk("getViewPreferences")
            tk("saveViewPreferences", {"expectedVersion": preferences["version"], "defaultView": preferences["defaultView"], "defaultProject": preferences["defaultProject"], "recent": preferences["recent"], "navigationPosition": "left"})
            browser("wait", "--fn", "preferences.navigationPosition==='left'")
            browser("eval", "if(terminalView!==window.navigationTerminal||document.documentElement.dataset.navigation!=='top')throw new Error('Remote navigation disturbed the attached terminal')")
            interventions = len(tk("listSessionInterventions", {"id": session_id})["items"])
            browser("press", "Escape")
            browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
            browser("wait", "--fn", "document.documentElement.dataset.navigation==='left'")
            assert len(tk("listSessionInterventions", {"id": session_id})["items"]) == interventions
            browser("wait", f'#session-icons [data-session="{session_id}"]')
            browser("click", f'#session-icons [data-session="{session_id}"]')
            browser("wait", ".xterm-screen")
            browser("screenshot", "/tmp/teleport-terminal.png")
            before_interrupt = len(tk("listSessionInterventions", {"id": session_id})["items"])
            browser("press", "Control+c")
            browser("wait", "--fn", "document.querySelector('tk-dialog[open] h2')?.textContent==='Interrupt harness'")
            browser("press", "Escape")
            assert len(tk("listSessionInterventions", {"id": session_id})["items"]) == before_interrupt
        process.terminate()
        process.wait(timeout=10)
        process = serve()
        assert tk("listSessions")["items"][0]["id"] == session_id
        second = {"id": session_id, "requestId": "terminal-after-restart", "data": base64.b64encode(b"after restart\n").decode()}
        assert tk("sendSessionInput", second)["state"] == "delivered"
        wait_for(lambda: ("native fixture answer resolved" if args.native_fixture else "received: after restart") in read())
        after = json.loads((folder / "runtime/sessions.json").read_text())[session_id]
        assert before["launchId"] == after["launchId"] and before["generation"] == after["generation"]
        assert row["directory"] != str(repository.resolve()) and row["directory"].startswith(str((folder / "worktrees").resolve()))
        tk("stopSession", {"id": session_id, "confirm": True})
        wait_for(lambda: tk("listSessions")["items"][0]["state"] == "exited")
        assert next(item for item in tk("listProjectSummaries")["items"] if item["id"] == project["id"])["agents"] == 0
        if args.native_fixture:
            expired = subprocess.run([str(ROOT / "bin/tk"), "listProjects"], env=context_env, capture_output=True, timeout=15)
            assert expired.returncode != 0, "Exited native context credential remained usable"
        settings = tk("getSession")["settings"]
        limited = tk("copyWorkflow", {"sourceKey": "agent@1", "projectId": project["id"], "title": "One implementation worker", "requestId": "native-quota"})
        for phase in limited["phases"]:
            if phase["role"] == "Doing":
                phase["workers"] = 1
                phase["description"] = "Keep native implementation inside the shared pool."
        limited = tk("saveWorkflow", {"definition": limited, "expectedRevision": limited["draftRevision"]})
        project = tk("activateWorkflowVersion", {"workflowKey": limited["id"] + "@draft", "expectedRevision": limited["draftRevision"], "projectId": project["id"], "expectedProjectVersion": project["version"]})
        settings = tk("setExecutionMode", {"mode": "demo", "expectedVersion": settings["version"]})
        ticket = tk("createTicket", {"projectId": project["id"], "title": "Prepared native task", "requestId": "native-task"})
        tk("requestRefinement", {"id": ticket["id"], "expectedVersion": ticket["version"]})
        def proposal():
            current = tk("getTicket", {"id": ticket["id"]})
            return current if current.get("refinement") else None
        ticket = wait_for(proposal)
        tk("setExecutionMode", {"mode": "off", "expectedVersion": settings["version"]})
        ticket = tk("acceptRefinement", {"id": ticket["id"], "expectedVersion": ticket["version"], "proposalId": ticket["refinement"]["id"]})
        ticket = tk("prepareTaskWorktree", {"id": ticket["id"], "expectedVersion": ticket["version"], "requestId": "prepare-native-task"})
        attached = tk("startTicketSession", {"projectId": project["id"], "ticketId": ticket["id"], "expectedVersion": ticket["version"], "harness": harness, "requestId": "ticket-native-start"})
        session_id = attached["id"]
        ticket = tk("getTicket", {"id": ticket["id"]})
        assert ticket["execution"] == "local:" + session_id and ticket["active"] and not ticket.get("attention")
        if not args.native_fixture:
            def board_active():
                result = tk("getBoardExecution", {"projectId": project["id"], "workflowKey": ticket["workflow"]})
                phase = next(row for row in result["phases"] if row["phaseId"] == ticket["phase"])
                return phase if phase["active"] == 1 else None
            observed_phase = wait_for(board_active)
            assert observed_phase["capacity"] == 1 and ticket["id"] in observed_phase["activeTicketIds"]
            if args.browser:
                browser("press", "Escape")
                browser("click", '#nav-views [data-view="Board"]')
                browser("wait", f'[data-ticket-card="{ticket["id"]}"].is-working')
                browser("eval", f"{{const node=document.querySelector('[data-ticket-card=\"{ticket['id']}\"] .agent-trace rect');if(getComputedStyle(node).animationName!=='agent-perimeter')throw new Error('Observed agent lacks a perimeter trace');}}")
                browser("screenshot", "/tmp/teleport-agent-perimeter.png")
                browser("wait", f'#session-icons [data-session="{session_id}"]')
                browser("click", f'#session-icons [data-session="{session_id}"]')
                browser("wait", ".xterm-screen")
                browser("press", "Escape")
            print("PASS: observed active/capacity, rounded perimeter trace and exact robot reconnect", flush=True)
        if args.native_fixture:
            question = wait_for(lambda: next((r for r in tk("getSessionRequests", {"id": session_id})["items"] if r["state"] == "pending"), None))
            wait_for(lambda: next((r for r in tk("listSessions")["items"] if r["id"] == session_id and r.get("attention")), None))
            if args.browser:
                browser("open", endpoint)
                browser("wait", "#welcome-status")
                browser("press", "Escape")
                browser("find", "role", "button", "click", "--name", "Board", "--exact")
                browser("wait", f'[data-ticket-card="{ticket["id"]}"] .needs-attention')
                browser("wait", "#harness-console.needs-attention")
                browser("eval", f'document.querySelector(\'[data-ticket-card="{ticket["id"]}"] .ticket-open\').focus()')
                browser("press", "Shift+F10")
                browser("wait", '[data-ticket-menu="teleport"]')
                interventions = len(tk("listSessionInterventions", {"id": session_id})["items"])
                browser("click", '[data-ticket-menu="teleport"]')
                browser("wait", ".xterm-screen")
                browser("eval", f'if(terminalView?.id!=={json.dumps(session_id)})throw new Error("Menu reconnected the wrong instance")')
                browser("press", "Escape")
                assert len(tk("listSessionInterventions", {"id": session_id})["items"]) == interventions
                browser("eval", f'document.querySelector(\'[data-ticket-card="{ticket["id"]}"] .ticket-open\').focus()')
                browser("press", "Shift+F10")
                browser("wait", '[data-ticket-menu="questions"]')
                browser("click", '[data-ticket-menu="questions"]')
                browser("wait", '[data-native-request]')
                # The custom select delegates its value to the hidden form field.
                browser("click", '#native-ticket-questions .choice-button')
                browser("find", "role", "option", "click", "--name", "Continue", "--exact")
                browser("fill", '[data-native-answer="secret"]', "fixture-secret")
                browser("click", '[data-native-send]')
                browser("wait", "--fn", "document.querySelector('.native-request h4')?.textContent.includes('resolved')")
                browser("wait", "--fn", f"document.querySelector('[data-session-summary=\"{session_id}\"] [data-session-label]')?.textContent.includes('Response ready')")
                browser("screenshot", "/tmp/teleport-native-question.png")
                browser("press", "Escape")
            else:
                answer = {"id": session_id, "requestId": question["id"], "operationId": "native-answer", "answers": {"direction": {"answers": ["Continue"]}, "secret": {"answers": ["fixture-secret"]}}}
                response = tk("respondToAgent", answer)
                assert response["state"] == "delivered" and tk("respondToAgent", answer) == response
            resolved = wait_for(lambda: next((r for r in tk("getSessionRequests", {"id": session_id})["items"] if r["state"] == "resolved"), None))
            assert resolved["source"] == "ticket/teleport dialog" and resolved["answers"]["secret"]["answers"] == ["[redacted]"]
        audit = tk("sendSessionInput", {"id": session_id, "requestId": "outside-ticket-input", "data": base64.b64encode(b"outside workflow\n").decode()})
        assert audit["state"] == "delivered" and audit["actor"] == "admin" and "data" not in audit
        if args.browser:
            browser("open", endpoint)
            browser("wait", "#welcome-status")
            browser("press", "Escape")
            browser("find", "role", "button", "click", "--name", "Board", "--exact")
            selector = f'[data-ticket-card="{ticket["id"]}"] [data-session="{session_id}"]'
            browser("wait", selector)
            browser("click", selector)
            browser("wait", ".xterm-screen")
            browser("press", "Escape")
        tk("stopSession", {"id": session_id, "confirm": True})
        wait_for(lambda: all(r["state"] in ("exited", "failed") for r in tk("listSessions")["items"]))
        ticket = tk("getTicket", {"id": ticket["id"]})
        assert not ticket["active"] and not ticket.get("execution") and ticket["status"] == "Doing"
        tk("shutdownLocalRuntime", {"confirm": True})
        clean = True
        print("PASS: real owned daemon/PTY, duplicate fencing, resize, same-instance API restart, observed stop and zero model calls" + ("; native wire question/resolution, redaction and scoped context credentials" if args.native_fixture else "") + ("; browser stdin/output, Escape detach and robot reconnect" if args.browser else ""))
    except Exception:
        if args.browser and browser_open:
            print(browser("eval", "JSON.stringify({error:document.querySelector('#terminal-error')?.textContent,active:document.activeElement?.outerHTML.slice(0,120),viewer:terminalView&&{cursor:terminalView.cursor,stopped:terminalView.stopped,sending:terminalView.sending,queue:terminalView.queue.length},state:document.querySelector('#terminal-state')?.textContent})"))
            browser("screenshot", "/tmp/teleport-terminal-failure.png")
        raise
    finally:
        if args.browser:
            subprocess.run(["agent-browser", "--session", browser_name, "close"], capture_output=True, timeout=15)
        if not clean and process is not None and process.poll() is None:
            try:
                if session_id:
                    tk("stopSession", {"id": session_id, "confirm": True})
                    wait_for(lambda: all(r["state"] in ("exited", "failed") for r in tk("listSessions")["items"]))
                tk("shutdownLocalRuntime", {"confirm": True})
                clean = True
            except (RuntimeError, AssertionError):
                print(f"Retain unresolved owned runtime evidence: {folder}")
        if process is not None and process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
        if clean:
            shutil.rmtree(folder)


if __name__ == "__main__":
    from tool_history import run
    run(main)
