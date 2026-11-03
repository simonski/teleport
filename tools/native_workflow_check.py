"""Exercise automatic local workflows in an isolated installation; zero providers."""
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

ROOT = Path(__file__).resolve().parents[1]


def main(database_url=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", help="Disposable empty PostgreSQL database")
    parser.add_argument("--browser", action="store_true")
    args = parser.parse_args()
    folder = Path(tempfile.mkdtemp(prefix="teleport-workflow-proof-"))
    fixture_bin = folder / "bin"
    fixture_bin.mkdir()
    subprocess.run(["go", "build", "-o", str(fixture_bin / "codex"), "./tools/fixtures/native-codex"], cwd=ROOT, check=True, timeout=60)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    env = dict(os.environ, TELEPORT_DB=args.database or database_url, TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
    env["PATH"] = str(fixture_bin) + os.pathsep + env["PATH"]
    process = None
    clean = False
    log = (folder / "server.log").open("w")
    browser_name = f"teleport-workflow-{port}"

    def tk(op, data=None):
        command = [str(ROOT / "bin/tk"), op, "-json"]
        if data is not None:
            command += ["--data", json.dumps(data)]
        result = subprocess.run(command, env=env, text=True, capture_output=True, timeout=35)
        if result.returncode:
            raise RuntimeError(f"{op}: {result.stderr}")
        return json.loads(result.stdout)

    def wait_for(callback, label):
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            value = callback()
            if value:
                return value
            time.sleep(0.1)
        raise AssertionError(f"Timed out: {label}; evidence: {folder}")

    def serve():
        server = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}", "--runtime-dir", str(folder / "runtime")], env=env, stdout=log, stderr=log)
        def ready():
            try:
                with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                    return response.status == 200
            except OSError:
                return False
        wait_for(ready, "server startup")
        return server

    def browser(*arguments):
        result = subprocess.run(["agent-browser", "--session", browser_name, *arguments], capture_output=True, text=True, timeout=35)
        if result.returncode:
            raise RuntimeError(result.stderr or result.stdout)
        return result.stdout

    try:
        subprocess.run([str(ROOT / "bin/tk"), "init"], env=env, check=True, capture_output=True, timeout=15)
        process = serve()
        subprocess.run([str(ROOT / "bin/tk"), "login", "--password-stdin"], env=env, input="password\n", text=True, check=True, capture_output=True, timeout=15)
        repo = folder / "repo"
        repo.mkdir()
        def git(*arguments):
            subprocess.run(["git", "-C", str(repo), "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.devnull, *arguments], check=True, capture_output=True, timeout=15)
        git("init", "-b", "main")
        (repo / "fixture.txt").write_text("original\n")
        git("add", ".")
        git("commit", "-m", "Seed")
        project = tk("createProject", {"name": "Native workflow", "requestId": "project"})
        project = tk("configureLocalRepository", {"projectId": project["id"], "expectedVersion": project["version"], "repository": str(repo), "storageRoot": str(folder / "worktrees")})
        ticket = tk("createTicket", {"projectId": project["id"], "title": "Update fixture", "requestId": "ticket"})
        ticket = tk("requestRefinement", {"id": ticket["id"], "expectedVersion": ticket["version"]})
        time.sleep(0.7)
        assert tk("listSessions")["items"] == [], "off mode launched a harness"
        settings = tk("getSession")["settings"]
        tk("setExecutionMode", {"mode": "local", "expectedVersion": settings["version"]})
        def sessions():
            return tk("listSessions")["items"]
        row = wait_for(lambda: next((r for r in sessions() if r["ticketId"] == ticket["id"] and r["state"] == "running"), None), "automatic Refine launch")
        first_id = row["id"]
        question = wait_for(lambda: next((r for r in tk("getSessionRequests", {"id": first_id})["items"] if r["state"] == "pending"), None), "native question")
        saved = tk("getPrompt", {"id": ticket["id"], "sessionId": first_id})
        assert saved["saved"] and [p["source"] for p in saved["prompt"]["contributions"]] == ["system", "project", "goal", "epic", "task", "phase"]
        assert row["ticketId"] == ticket["id"]
        process.terminate()
        process.wait(timeout=10)
        process = serve()
        assert sessions()[0]["id"] == first_id, "API restart replaced a live phase"
        assert tk("getPrompt", {"id": ticket["id"], "sessionId": first_id}) == saved
        if args.browser:
            browser("open", endpoint)
            browser("wait", '[name="username"]')
            browser("fill", '[name="username"]', "admin")
            browser("fill", '[name="password"]', "password")
            browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
            browser("wait", "#welcome-status")
            browser("press", "Escape")
            browser("find", "role", "button", "click", "--name", "Board", "--exact")
            browser("wait", f'[data-ticket-card="{ticket["id"]}"]')
            browser("eval", f'document.querySelector(\'[data-ticket-card="{ticket["id"]}"] .ticket-open\').focus()')
            browser("press", "Shift+F10")
            browser("click", '[data-ticket-menu="prompt"]')
            browser("wait", "pre.criteria-box")
            browser("eval", f'if(!document.querySelector("#editor").textContent.includes("Saved launch · {first_id}"))throw new Error("Prompt did not preserve the exact managed session")')
            browser("screenshot", "/tmp/teleport-native-prompt.png")
            browser("press", "Escape")
            browser("eval", f"editTicket({ticket['id']})")
            browser("wait", "[data-native-request]")
            browser("click", "#native-ticket-questions .choice-button")
            browser("find", "role", "option", "click", "--name", "Continue", "--exact")
            browser("fill", '[data-native-answer="secret"]', "fixture-secret")
            browser("click", "[data-native-send]")
            browser("press", "Escape")
        else:
            tk("respondToAgent", {"id": first_id, "requestId": question["id"], "operationId": "answer", "answers": {"direction": {"answers": ["Continue"]}, "secret": {"answers": ["fixture-secret"]}}})
        def current():
            return tk("getTicket", {"id": ticket["id"]})
        ticket = wait_for(lambda: (r if r.get("refinement") and not r.get("execution") else None) if (r := current()) else None, "Refine proposal after confirmed exit")
        assert ticket["status"] == "Refine" and not ticket["refinement"].get("acceptedBy")
        assert (repo / "fixture.txt").read_text() == "original\n"
        if args.browser:
            browser("eval", f"editTicket({ticket['id']})")
            browser("wait", '[data-agent="accept"]')
            browser("click", '[data-agent="accept"]')
            browser("click", '.checkbox-label')
            browser("find", "role", "button", "click", "--name", "Accept scope", "--exact")
        else:
            ticket = tk("acceptRefinement", {"id": ticket["id"], "expectedVersion": ticket["version"], "proposalId": ticket["refinement"]["id"]})
        ticket = wait_for(lambda: (r if r["status"] == "Approval" and not r.get("execution") else None) if (r := current()) else None, "automatic Implementation and separate Review")
        assert ticket["agentReview"]["outcome"] == "success" and not ticket["agentReview"]["simulated"]
        assert ticket["agentReview"]["candidate"] == ticket["delivery"]["candidate"]["commit"]
        assert (repo / "fixture.txt").read_text() == "original\n", "agent integrated main without approval"
        all_rows = sessions()
        assert len(all_rows) == 3 and len({r["id"] for r in all_rows}) == 3
        assert all(r["state"] in ("exited", "failed") for r in all_rows)
        assert ticket["attempts"] == 3
        assert [e["phase"] for e in ticket["stageEvents"]] == ["Refine", "Doing", "Review"]
        if args.browser:
            browser("eval", f"editTicket({ticket['id']})")
            browser("wait", ".scope-proposal")
            browser("screenshot", "/tmp/teleport-native-workflow.png")
            browser("click", '[data-delivery="approve"]')
            browser("click", '.checkbox-label')
            browser("find", "role", "button", "click", "--name", "Approve and fast-forward main", "--exact")
            ticket = wait_for(lambda: (r if r["status"] == "Done" else None) if (r := current()) else None, "human browser approval")
        else:
            ticket = tk("approveTicket", {"id": ticket["id"], "expectedVersion": ticket["version"], "confirm": True})
        assert ticket["status"] == "Done" and (repo / "fixture.txt").read_text() == "delivered\n"
        tk("shutdownLocalRuntime", {"confirm": True})
        clean = True
        print("PASS: automatic Refine → human scope acceptance → Implementation → separate Review → human Approval; saved prompts, native question, same-instance restart, finite attempts, confirmed exits; zero provider calls")
    finally:
        if args.browser:
            subprocess.run(["agent-browser", "--session", browser_name, "close"], capture_output=True, timeout=15)
        if not clean and process is not None and process.poll() is None:
            try:
                settings = tk("getSession")["settings"]
                tk("setExecutionMode", {"mode": "off", "expectedVersion": settings["version"]})
                for row in tk("listSessions")["items"]:
                    if row["state"] not in ("exited", "failed"):
                        tk("stopSession", {"id": row["id"], "confirm": True})
                wait_for(lambda: all(r["state"] in ("exited", "failed") for r in sessions()), "cleanup")
                tk("shutdownLocalRuntime", {"confirm": True})
                clean = True
            except (RuntimeError, AssertionError):
                print(f"Retain unresolved runtime evidence: {folder}")
        if process is not None and process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
        log.close()
        if clean:
            shutil.rmtree(folder)
        else:
            print(f"Retained workflow evidence: {folder}")


if __name__ == "__main__":
    from tool_history import run
    from postgres import run_with_database
    run(lambda: run_with_database(main))
