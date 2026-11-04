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


def main(database_url=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", action="store_true")
    parser.add_argument("--native-fixture", action="store_true", help="Exercise native Codex protocol through a zero-provider fake executable")
    parser.add_argument("--native-large-frame", action="store_true", help="Exercise a large native protocol frame with the fixture")
    parser.add_argument("--shell-pty", action="store_true", help="Exercise a real local zsh PTY without an agent")
    parser.add_argument("--codex-pty", action="store_true", help="Exercise direct Codex PTY with a zero-model fixture")
    parser.add_argument("--harness-pty", choices=["claude", "copilot"], help="Exercise a direct harness PTY with a zero-model fixture")
    parser.add_argument("--console-only", action="store_true", help="Check only the independent project-root console")
    parser.add_argument("--native-startup-only", action="store_true", help="Check installed Codex startup without provider credentials or a model turn")
    parser.add_argument("--database", help="Existing empty disposable PostgreSQL database")
    args = parser.parse_args()
    folder = Path(tempfile.mkdtemp(prefix="teleport-terminal-proof-"))
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    environment = dict(os.environ, TELEPORT_DB=args.database or database_url, TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
    if args.codex_pty:
        isolated_home = folder / "home"
        isolated_home.mkdir(mode=0o700)
        environment["HOME"] = str(isolated_home)
    if args.shell_pty:
        shell_home = folder / "shell-config"
        shell_home.mkdir(mode=0o700)
        environment["ZDOTDIR"] = str(shell_home)
    if args.native_fixture or args.codex_pty or args.harness_pty:
        fixture_bin = folder / "fixture-bin"
        fixture_bin.mkdir()
        fixture_name = args.harness_pty or "codex"
        subprocess.run(["go", "build", "-o", str(fixture_bin / fixture_name), "./tools/fixtures/native-codex"], cwd=ROOT, check=True, timeout=60)
        environment["PATH"] = str(fixture_bin) + os.pathsep + environment["PATH"]
        if args.native_large_frame:
            environment["TK_NATIVE_FIXTURE_LARGE"] = "1"
    if args.native_fixture and args.native_startup_only:
        parser.error("choose one native fixture")
    if args.native_startup_only:
        args.console_only = True
        environment = {key: value for key, value in environment.items() if not key.startswith("OPENAI_") and key not in ("CODEX_API_KEY", "CODEX_HOME")}
        environment["CODEX_HOME"] = str(folder / "isolated-codex")
        (folder / "isolated-codex").mkdir(mode=0o700)
    harness = "codex" if args.native_fixture or args.native_startup_only else "fixture"
    process = None
    host_fixture = None
    session_id = None
    clean = False
    browser_name = f"teleport-terminal-{port}"
    browser_open = False

    def tk(operation, data=None):
        command = [str(ROOT / "bin/tk"), operation, "-json"]
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
        def git_output(root, *arguments):
            return subprocess.check_output(["git", "-C", str(root), *arguments], text=True, timeout=15)
        def git(*arguments):
            subprocess.run(["git", "-c", f"core.hooksPath={os.devnull}", "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "-C", str(repository), *arguments], check=True, capture_output=True, timeout=15)
        git("init", "-b", "main")
        (repository / "fixture.txt").write_text("owned terminal proof\n")
        git("add", "fixture.txt")
        git("commit", "-m", "Seed terminal fixture")
        project = tk("createProject", {"name": "Terminal proof", "key": "terminal-" + uuid.uuid4().hex[:12], "requestId": "terminal-project"})
        project = tk("configureLocalRepository", {"projectId": project["id"], "expectedVersion": project["version"], "repository": str(repository), "storageRoot": str(folder / "worktrees")})
        if args.shell_pty or args.codex_pty or args.harness_pty:
            capacity = {"projectId": project["id"], "expectedVersion": project["version"], "refineWorkers": 1, "doingWorkers": 1, "maxAgents": 0 if args.shell_pty else 2}
            if args.codex_pty or args.harness_pty:
                capacity["allowedHarnesses"] = [args.harness_pty or "codex"]
            project = tk("setLaneCapacity", capacity)
        if args.native_startup_only:
            config = folder / "isolated-codex" / "config.toml"
            config.write_text("[projects." + json.dumps(str(folder.resolve())) + "]\ntrust_level = \"untrusted\"\n[projects." + json.dumps(str(repository.resolve())) + "]\ntrust_level = \"trusted\"\n")
        if args.console_only:
            git("checkout", "-b", "console-feature")
        operation = "openShellSession" if args.shell_pty else "openCodexTerminal" if args.codex_pty else "open" + args.harness_pty.title() + "Terminal" if args.harness_pty else "openConsoleSession"
        start = {"projectId": project["id"], "requestId": "terminal-start"}
        if not args.shell_pty and not args.codex_pty and not args.harness_pty:
            start["harness"] = harness
        row = tk(operation, start)
        session_id = row["id"]
        assert row["state"] == "running" and row["ticketId"] == 0
        assert tk(operation, start)["id"] == session_id
        import base64
        def read():
            return base64.b64decode(tk("attachSession", {"id": session_id}).get("data", "")).decode(errors="replace")
        if args.codex_pty or args.harness_pty:
            direct_name = args.harness_pty or "codex"
            setup = tk("getSession")["welcomeSetup"]
            if not args.browser:
                tk("setDefaultHarness", {"expectedVersion": setup["version"], "harness": direct_name})
            assert row["harness"] == direct_name + "-pty" and row["directory"] == str(repository.resolve())
            if args.codex_pty:
                skill_path = isolated_home / ".agents/skills/teleport/SKILL.md"
                assert skill_path.read_text() == subprocess.check_output([str(ROOT / "bin/tk"), "skill"], env=environment, text=True, timeout=10)
            manifest = json.loads((folder / "runtime/sessions.json").read_text())[session_id]
            assert not manifest.get("native") and not (folder / "runtime" / (session_id + ".client.json")).exists()
            wait_for(lambda: "direct Codex fixture ready" in read())
            data = base64.b64encode(b"hello\n").decode()
            assert tk("sendSessionInput", {"id": session_id, "requestId": "direct-input", "data": data})["state"] == "delivered"
            wait_for(lambda: "direct fixture received: hello" in read())
            tk("resizeSession", {"id": session_id, "cols": 120, "rows": 35})
            if args.browser:
                fixture_dir = folder / "host-fixture"
                fixture_dir.mkdir()
                subprocess.run(["go", "build", "-o", str(fixture_dir / "codex"), "./tools/fixtures/native-codex"], cwd=ROOT, check=True, timeout=60)
                host_fixture = subprocess.Popen([str(fixture_dir / "codex"), "app-server"], stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                wait_for(lambda: next((item for item in tk("listHostHarnesses")["items"] if item["pid"] == host_fixture.pid and not item["managed"]), None))
                browser("open", endpoint)
                browser_open = True
                browser("find", "label", "Username", "fill", "admin")
                browser("find", "label", "Password", "fill", "password")
                browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
                browser("wait", "#welcome-status")
                browser("press", "Escape")
                browser("click", "#account")
                browser("click", "#account-settings")
                browser("select", "#default-harness", direct_name)
                browser("click", "#save-default-harness")
                browser("wait", "--fn", "document.querySelector('#default-harness-status')?.textContent==='Default harness saved.'")
                browser("press", "Escape")
                browser("click", f"#{direct_name}-pty-console")
                browser("wait", ".xterm-screen")
                second = wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] != session_id and item["harness"] == direct_name + "-pty"), None))
                wait_for(lambda: "direct Codex fixture ready" in base64.b64decode(tk("attachSession", {"id": second["id"]}).get("data", "")).decode(errors="replace"))
                blocked = subprocess.run([str(ROOT / "bin/tk"), operation, "--data", json.dumps({"projectId": project["id"], "requestId": "over-limit"})], env=environment, text=True, capture_output=True, timeout=15)
                assert blocked.returncode != 0 and "limit" in blocked.stderr.lower(), blocked.stderr
                for key in "quit":
                    browser("press", key)
                browser("press", "Enter")
                browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] == second["id"] and item["state"] == "exited"), None))
                browser("click", "#harness-console")
                browser("wait", "--fn", f"terminalView?.id==={json.dumps(session_id)}")
                browser("press", "Escape")
                tk("stopSession", {"id": session_id, "confirm": True})
                wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] == session_id and item["state"] == "exited"), None))
                browser("click", "#harness-console")
                browser("wait", ".xterm-screen")
                third = wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] not in (session_id, second["id"]) and item["harness"] == direct_name + "-pty"), None))
                wait_for(lambda: "direct Codex fixture ready" in base64.b64decode(tk("attachSession", {"id": third["id"]}).get("data", "")).decode(errors="replace"))
                for key in "quit":
                    browser("press", key)
                browser("press", "Enter")
                browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] == third["id"] and item["state"] == "exited"), None))
                fourth = tk(operation, {"projectId": project["id"], "requestId": "session-list-stop"})
                browser("click", "#session-recovery")
                browser("wait", f'[data-host-inspect="{host_fixture.pid}"]')
                browser("click", f'[data-host-inspect="{host_fixture.pid}"]')
                browser("wait", "--fn", "document.querySelector('tk-dialog[open] h2')?.textContent==='Harness process'")
                browser("click", "#editor-form button[type=submit]")
                browser("wait", "--fn", "document.querySelector('tk-dialog[open] h2')?.textContent==='Harness sessions'")
                browser("wait", f'[data-host-stop="{host_fixture.pid}"]')
                browser("click", f'[data-host-stop="{host_fixture.pid}"]:not([data-force])')
                browser("click", "#editor-form button[type=submit]")
                browser("wait", "--fn", "document.querySelector('tk-dialog[open] h2')?.textContent==='Harness sessions'")
                host_fixture.wait(timeout=10)
                browser("wait", "--fn", f"!document.querySelector('[data-host-stop=\"{host_fixture.pid}\"]')")
                browser("click", f'[data-stop-managed="{fourth["id"]}"]')
                browser("click", "#editor-form button[type=submit]")
                browser("wait", "--fn", "document.querySelector('tk-dialog[open] h2')?.textContent==='Harness sessions'")
                browser("wait", "--fn", f"!document.querySelector('[data-stop-managed=\"{fourth['id']}\"]')")
                browser("press", "Escape")
            tk("stopSession", {"id": session_id, "confirm": True})
            wait_for(lambda: all(item["state"] in ("exited", "failed") for item in tk("listSessions")["items"]))
            tk("shutdownLocalRuntime", {"confirm": True})
            clean = True
            print(f"PASS: direct {direct_name} fixture PTY, input/output, resize and owned exit; no model calls" + ("; browser start and quit close dialog" if args.browser else ""))
            return
        if args.shell_pty:
            assert row["harness"] == "shell" and row["directory"] == str(repository.resolve())
            command = b"printf 'TK_%s\\n' SHELL_READY\n"
            receipt = tk("sendSessionInput", {"id": session_id, "requestId": "shell-command", "data": base64.b64encode(command).decode()})
            assert receipt["state"] == "delivered"
            wait_for(lambda: "TK_SHELL_READY" in read())
            tk("resizeSession", {"id": session_id, "cols": 120, "rows": 35})
            second = tk("openShellSession", {"projectId": project["id"], "requestId": "second-shell"})
            assert second["id"] != session_id and second["state"] == "running"
            assert next(item for item in tk("listProjectSummaries")["items"] if item["id"] == project["id"])["agents"] == 0
            if args.browser:
                browser("open", endpoint)
                browser_open = True
                browser("find", "label", "Username", "fill", "admin")
                browser("find", "label", "Password", "fill", "password")
                browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
                browser("wait", "#welcome-status")
                browser("press", "Escape")
                browser("click", "#shell-console")
                browser("wait", ".xterm-screen")
                third = wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] not in (session_id, second["id"]) and item["harness"] == "shell"), None))
                browser("eval", "terminalView.terminal.paste(\"printf 'TK_%s\\\\n' BROWSER_READY\");true")
                browser("press", "Enter")
                def browser_output():
                    output = tk("attachSession", {"id": third["id"]})["data"]
                    return "TK_BROWSER_READY" in base64.b64decode(output).decode(errors="replace")
                wait_for(browser_output)
                for key in "exit":
                    browser("press", key)
                browser("press", "Enter")
                browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                wait_for(lambda: next((item for item in tk("listSessions")["items"] if item["id"] == third["id"] and item["state"] == "exited"), None))
            tk("stopSession", {"id": second["id"], "confirm": True})
            tk("stopSession", {"id": session_id, "confirm": True})
            wait_for(lambda: all(item["state"] in ("exited", "failed") for item in tk("listSessions")["items"]))
            tk("shutdownLocalRuntime", {"confirm": True})
            clean = True
            print("PASS: independent project-root zsh PTYs, input/output, resize and confirmed stop with zero agent capacity" + ("; browser shell control, input and dialog close after exit" if args.browser else ""))
            return
        if args.native_startup_only:
            def native_ready():
                screen = read()
                if "remote project directory is inside an explicitly untrusted project" in screen:
                    raise AssertionError("Native Codex still rejected the project trust scope")
                return screen if any(word in screen.lower() for word in ("sign in", "log in", "welcome", "codex")) else None
            wait_for(native_ready)
            for _ in range(5):
                time.sleep(.2)
                assert "remote project directory is inside an explicitly untrusted project" not in read()
                assert tk("listSessions")["items"][0]["state"] == "running"
            assert row["directory"] == str(repository.resolve())
            assert git_output(repository, "worktree", "list", "--porcelain").count("worktree ") == 1
            tk("stopSession", {"id": session_id, "confirm": True})
            wait_for(lambda: tk("listSessions")["items"][0]["state"] in ("exited", "failed"))
            tk("shutdownLocalRuntime", {"confirm": True})
            clean = True
            print("PASS: installed Codex starts in a trusted fixture project without a provider credential or model turn")
            return
        wait_for(lambda: ("native fixture connected" if args.native_fixture else "terminal fixture") in read())
        if args.native_large_frame:
            wait_for(lambda: "native fixture large frame received" in read())
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
        assert row["directory"] == str(repository.resolve()), "Independent console did not use its project root"
        assert git_output(repository, "worktree", "list", "--porcelain").count("worktree ") == 1, "Independent console created a worktree"
        tk("stopSession", {"id": session_id, "confirm": True})
        wait_for(lambda: tk("listSessions")["items"][0]["state"] == "exited")
        assert next(item for item in tk("listProjectSummaries")["items"] if item["id"] == project["id"])["agents"] == 0
        if args.native_fixture:
            expired = subprocess.run([str(ROOT / "bin/tk"), "listProjects"], env=context_env, capture_output=True, timeout=15)
            assert expired.returncode != 0, "Exited native context credential remained usable"
        if args.console_only:
            tk("shutdownLocalRuntime", {"confirm": True})
            clean = True
            print("PASS: project-root console, native directory argument, reconnect and observed stop; zero model calls")
            return
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
        if host_fixture is not None and host_fixture.poll() is None:
            host_fixture.terminate()
            host_fixture.wait(timeout=10)
        if args.browser:
            subprocess.run(["agent-browser", "--session", browser_name, "close"], capture_output=True, timeout=15)
        if not clean and process is not None and process.poll() is None:
            try:
                if session_id:
                    for item in tk("listSessions")["items"]:
                        if item["state"] not in ("exited", "failed"):
                            tk("stopSession", {"id": item["id"], "confirm": True})
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
    from postgres import run_with_database
    run(lambda: run_with_database(main))
