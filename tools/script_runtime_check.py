"""Verify owned isolated JavaScript jobs and human gates without model calls."""
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
from script_prompt_browser import verify_script_prompt

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", action="store_true")
    parser.add_argument("--database")
    args = parser.parse_args()
    folder = Path(tempfile.mkdtemp(prefix="teleport-script-proof-"))
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    environment = dict(os.environ, TELEPORT_DB=args.database or str(folder / "database.db"), TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
    process = None
    clean = False
    browser_open = False
    browser_name = f"teleport-script-{port}"

    def tk(operation, data=None, fail=False):
        command = [str(ROOT / "bin/tk"), operation]
        if data is not None:
            command += ["--data", json.dumps(data)]
        result = subprocess.run(command, env=environment, text=True, capture_output=True, timeout=20)
        if fail:
            assert result.returncode != 0, result.stdout
            return result.stderr
        if result.returncode:
            raise RuntimeError(result.stderr)
        return json.loads(result.stdout)

    def wait_for(callback, timeout=15):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            result = callback()
            if result:
                return result
            time.sleep(0.05)
        raise AssertionError("Script runtime proof timed out")

    def serve():
        server = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}", "--runtime-dir", str(folder / "runtime")], env=environment, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        def ready():
            try:
                with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                    return response.status == 200
            except OSError:
                return False
        wait_for(ready)
        return server

    def login():
        subprocess.run([str(ROOT / "bin/tk"), "login", "--password-stdin"], env=environment, input="password\n", text=True, check=True, capture_output=True, timeout=20)

    def browser(*arguments):
        result = subprocess.run(["agent-browser", "--session", browser_name, *arguments], text=True, capture_output=True, timeout=40)
        if result.returncode:
            raise RuntimeError(result.stderr or result.stdout)
        return result.stdout

    def completed(identity):
        view = tk("getScriptRun", {"id": identity})
        return view if view["run"]["state"] in ("completed", "failed", "interrupted", "unknown", "stopped") else None

    def start(source, request_id=None, mode="script"):
        return tk("startScriptRun", {"source": source, "mode": mode, "requestId": request_id or uuid.uuid4().hex})

    try:
        subprocess.run([str(ROOT / "bin/tk"), "init"], env=environment, check=True, capture_output=True, timeout=20)
        process = serve()
        login()
        project = tk("createProject", {"name": "Script proof", "key": "script-" + uuid.uuid4().hex[:12], "requestId": "script-project"})
        source = f'const ids=[];for(let i=0;i<3;i++){{const ticket=await tk("createTicket",{{projectId:{project["id"]},title:"Script ticket "+i,agent:false,requestId:"script-create-"+i}});ids.push(ticket.id)}}console.log("created",ids.length);return ids;'
        row = start(source, "first-script")
        assert start(source, "first-script")["id"] == row["id"]
        view = wait_for(lambda: completed(row["id"]))
        assert view["run"]["state"] == "completed", view
        assert len(view["calls"]) == 3 and view["output"]["logs"] == ["created 3"] and len(view["output"]["value"]) == 3
        assert "different source" in tk("startScriptRun", {"source": "return 9;", "mode": "script", "requestId": "first-script"}, fail=True)
        assert tk("listSessions")["items"] == [], "Script jobs fabricated coding-harness sessions"
        ambient = start('[typeof process,typeof require,typeof fetch,typeof document,typeof std,typeof os]', mode="expression")
        ambient = wait_for(lambda: completed(ambient["id"]))
        assert ambient["output"]["value"] == ["undefined"] * 6
        long_run = start(f'await tk("createTicket",{{projectId:{project["id"]},title:"Before stop",agent:false,requestId:"before-stop"}});while(true){{}}')
        wait_for(lambda: tk("getScriptRun", {"id": long_run["id"]})["calls"])
        tk("stopScriptRun", {"id": long_run["id"], "confirm": True})
        stopped = wait_for(lambda: completed(long_run["id"]))
        assert stopped["run"]["state"] == "stopped" and stopped["calls"][0]["state"] == "completed", stopped
        settings = tk("getSession")["settings"]
        settings = tk("setExecutionMode", {"mode": "demo", "expectedVersion": settings["version"]})
        ticket = tk("createTicket", {"projectId": project["id"], "title": "Human-gated script scope", "requestId": "script-scope"})
        tk("requestRefinement", {"id": ticket["id"], "expectedVersion": ticket["version"]})
        ticket = wait_for(lambda: (lambda t: t if t.get("refinement") else None)(tk("getTicket", {"id": ticket["id"]})))
        other_ticket = tk("createTicket", {"projectId": project["id"], "title": "Other gated scope", "requestId": "other-script-scope"})
        tk("requestRefinement", {"id": other_ticket["id"], "expectedVersion": other_ticket["version"]})
        other_ticket = wait_for(lambda: (lambda t: t if t.get("refinement") else None)(tk("getTicket", {"id": other_ticket["id"]})))
        tk("setExecutionMode", {"mode": "off", "expectedVersion": settings["version"]})
        acceptance = {"id": ticket["id"], "expectedVersion": ticket["version"], "proposalId": ticket["refinement"]["id"]}
        row = start('return (await tk("acceptRefinement",' + json.dumps(acceptance) + ')).status;')
        wait_for(lambda: tk("getScriptRun", {"id": row["id"]})["run"]["state"] == "waiting")
        other_acceptance = {"id": other_ticket["id"], "expectedVersion": other_ticket["version"], "proposalId": other_ticket["refinement"]["id"]}
        other_run = start('return (await tk("acceptRefinement",' + json.dumps(other_acceptance) + ')).status;')
        wait_for(lambda: tk("getScriptRun", {"id": other_run["id"]})["run"]["state"] == "waiting")
        before = json.loads((folder / "runtime/sessions.json").read_text())[row["id"]]
        tk("logout")
        login()
        assert tk("getScriptRun", {"id": row["id"]})["run"]["state"] == "waiting"
        process.terminate()
        process.wait(timeout=10)
        process = serve()
        assert tk("getScriptRun", {"id": row["id"]})["run"]["state"] == "waiting"
        if args.browser:
            browser("open", endpoint)
            browser_open = True
            browser("find", "label", "Username", "fill", "admin")
            browser("find", "label", "Password", "fill", "password")
            browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
            browser("wait", "#welcome-status")
            browser("press", "Escape")
            browser("press", "/")
            browser("wait", "#script-approve")
            # Delay only a data read in this owned browser fixture. Source
            # remains data and is never evaluated in the page.
            browser("eval", "globalThis.__scriptProofApi=api;globalThis.__scriptProofHeld=false;globalThis.__scriptProofRelease=null;api=async function(op,data){const result=await globalThis.__scriptProofApi(op,data);if(op==='getScriptRun'&&data.id===" + json.dumps(row["id"]) + "&&!globalThis.__scriptProofHeld){globalThis.__scriptProofHeld=true;await new Promise(resolve=>globalThis.__scriptProofRelease=resolve);}return result;};")
            browser("click", "#script-history + .choice-button")
            browser("find", "role", "option", "click", "--name", row["id"] + " · waiting", "--exact")
            browser("wait", "--fn", "typeof globalThis.__scriptProofRelease==='function'")
            draft = 'return "edited while loading";'
            browser("fill", "#script-source", draft)
            browser("eval", "api=globalThis.__scriptProofApi;globalThis.__scriptProofRelease();")
            browser("wait", "--fn", "document.querySelector('#script-confirmation')?.textContent.includes(" + json.dumps("#" + str(ticket["id"]) + " ") + ")")
            assert draft in browser("get", "value", "#script-source"), "Delayed read overwrote edited source"
            browser("press", "Escape")
            assert tk("getScriptRun", {"id": row["id"]})["run"]["state"] == "waiting"
            browser("press", "/")
            browser("wait", "#script-approve")
            browser("click", "#script-approve")
            browser("wait", "--fn", "document.querySelector('#script-status')?.textContent.includes('completed')")
            browser("screenshot", "/tmp/teleport-script-console.png")
            browser("press", "Escape")
        else:
            pending = tk("getScriptRun", {"id": row["id"]})["calls"][0]
            tk("confirmScriptCall", {"id": row["id"], "callId": pending["id"], "fingerprint": pending["fingerprint"], "decision": "approve", "confirm": True})
        finished = wait_for(lambda: completed(row["id"]))
        assert finished["run"]["state"] == "completed" and finished["output"]["value"] == "Ready", finished
        assert finished["calls"][0]["confirmationActor"] == "admin"
        assert tk("getScriptRun", {"id": other_run["id"]})["run"]["state"] == "waiting", "Confirmation applied to the wrong run"
        assert tk("getTicket", {"id": other_ticket["id"]})["status"] == "Refine"
        tk("stopScriptRun", {"id": other_run["id"], "confirm": True})
        wait_for(lambda: completed(other_run["id"]))
        manual = tk("createTicket", {"projectId": project["id"], "title": "Manual completion gate", "criteria": "Fixture completion criteria", "agent": False, "requestId": "script-human-done"})
        manual = tk("transitionTicket", {"id": manual["id"], "expectedVersion": manual["version"], "target": "Doing"})
        done_input = {"id": manual["id"], "expectedVersion": manual["version"], "target": "Done", "confirm": True}
        done_run = start('return (await tk("transitionTicket",' + json.dumps(done_input) + ')).status;')
        wait_for(lambda: tk("getScriptRun", {"id": done_run["id"]})["run"]["state"] == "waiting")
        if args.browser:
            browser("press", "/")
            browser("wait", "#script-source")
            browser("click", "#script-history + .choice-button")
            browser("find", "role", "option", "click", "--name", done_run["id"] + " · waiting", "--exact")
            browser("wait", "#script-approve")
            assert "Fixture completion criteria" in browser("get", "text", "#script-confirmation")
            browser("click", "#script-approve")
            browser("wait", "--fn", "document.querySelector('#script-status')?.textContent.includes('completed')")
            browser("screenshot", "/tmp/teleport-script-console.png")
            browser("press", "Escape")
        else:
            pending = tk("getScriptRun", {"id": done_run["id"]})["calls"][0]
            tk("confirmScriptCall", {"id": done_run["id"], "callId": pending["id"], "fingerprint": pending["fingerprint"], "decision": "approve", "confirm": True})
        done = wait_for(lambda: completed(done_run["id"]))
        assert done["run"]["state"] == "completed" and done["output"]["value"] == "Done", done
        if args.browser:
            verify_script_prompt(browser, tk, project["id"])
        after = json.loads((folder / "runtime/sessions.json").read_text())[row["id"]]
        assert before["generation"] == after["generation"] and before["launchId"] == after["launchId"]
        tk("shutdownLocalRuntime", {"confirm": True})
        clean = True
        print("PASS: owned isolated JavaScript, language/async API calls, duplicate receipts, private run credentials, bounded stop, logout/API restart and exact human confirmation; zero model calls" + ("; slash/Escape/reconnect browser path" if args.browser else ""))
    except Exception:
        if browser_open:
            print(browser("snapshot", "-i"))
            browser("screenshot", "/tmp/teleport-script-failure.png")
        raise
    finally:
        if browser_open:
            subprocess.run(["agent-browser", "--session", browser_name, "close"], capture_output=True, timeout=20)
        if not clean and process is not None and process.poll() is None:
            try:
                for run in tk("listScriptRuns")["items"]:
                    if run["state"] in ("starting", "running", "waiting", "stopping", "unknown"):
                        tk("stopScriptRun", {"id": run["id"], "confirm": True})
                wait_for(lambda: all(run["state"] not in ("starting", "running", "waiting", "stopping", "unknown") for run in tk("listScriptRuns")["items"]))
                tk("shutdownLocalRuntime", {"confirm": True})
                clean = True
            except (RuntimeError, AssertionError):
                print(f"Retain unresolved owned script evidence: {folder}")
        if process is not None and process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
        if clean:
            shutil.rmtree(folder)


if __name__ == "__main__":
    from tool_history import run
    run(main)
