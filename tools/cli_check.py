"""Exercise the real tk command catalogue and text capture against an owned zero-model HTTP fixture."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import select
import subprocess
import tempfile
import threading
import time

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=ROOT / "bin/tk")
    args = parser.parse_args()
    receipts, calls, failures, old_server = {}, [], [], [False]

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *arguments):
            pass

        def do_POST(self):
            operation = self.path.rsplit("/", 1)[-1]
            calls.append(operation)
            if operation != "login" and self.headers.get("Authorization") != "Bearer cli-fixture-token":
                failures.append("Current credentials were not retained")
            data = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            if operation == "login":
                if data != {"username": "pilot", "password": "fixture-secret"}:
                    failures.append("Login changed supplied credentials")
                result = {"token": "cli-fixture-token", "account": {"username": "pilot"}}
            elif operation == "getSession":
                result = {"account": {"id": "fixture"}}
            elif operation == "listProjects":
                result = {"items": [{"id": 2, "name": "Flight"}]}
            elif operation == "listSessions":
                if data != {"projectId": 2}:
                    failures.append("Agent list changed project scope")
                result = {"items": [], "nextCursor": ""}
            elif operation == "saveIdea":
                if old_server[0] and "origin" in data:
                    body = json.dumps({"error": {"code": "invalid_request", "message": "Invalid request fields or JSON"}}).encode()
                    self.send_response(400)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                key = data["requestId"]
                if key in receipts and receipts[key][0] != data:
                    failures.append("A retry changed capture content")
                if key not in receipts:
                    receipts[key] = (data, {**data, "id": "idea:" + str(len(receipts) + 1), "version": 1})
                result = receipts[key][1]
            else:
                failures.append("Unexpected operation: " + operation)
                result = {"error": {"message": "Unexpected fixture operation"}}
            body = json.dumps(result).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    fixture = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=lambda: fixture.serve_forever(poll_interval=.05), daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="tk-cli-proof-") as temporary:
            folder = Path(temporary)
            endpoint = "http://127.0.0.1:" + str(fixture.server_port)
            config = folder / "client.json"
            config.write_text(json.dumps({"server": endpoint, "token": "cli-fixture-token"}))
            config.chmod(0o600)
            environment = dict(os.environ, TELEPORT_CONFIG=str(config), TELEPORT_SERVER=endpoint)

            def tk(*arguments, input=None, okay=True):
                result = subprocess.run([str(args.binary), *arguments], input=input, env=environment, capture_output=True, text=True, timeout=5)
                if okay and result.returncode:
                    raise AssertionError("CLI fixture command failed: " + result.stderr)
                if not okay and result.returncode == 0:
                    raise AssertionError("Invalid command succeeded")
                return result

            catalogue = json.loads(tk("commands", "-json").stdout)["commands"]
            assert {"agent ls", "ticket show", "project ls", "import -project NAME [FILE | -i]"} <= {item["command"] for item in catalogue}
            assert len(catalogue) > 100 and not calls
            overview = tk().stdout
            assert "\n  agent " in overview and "\n  ticket " in overview and "\n  tk " not in overview and "\n  agent ls " not in overview
            agent_help = tk("agent").stdout
            assert "tk agent ls " in agent_help and "tk ticket show " not in agent_help and not calls
            assert "-project" in tk("agent", "ls", "-h").stdout and not calls
            assert json.loads(tk("agent", "ls", "-project", "2", "-json").stdout)["items"] == []
            assert json.loads(tk("agent", "ls", "-f", "-", "-json", input='{"projectId":2}').stdout)["items"] == []
            before = len(calls)
            for name in ("init", "serve", "_session-daemon"):
                tk("api", name, okay=False)
            assert len(calls) == before
            invalid = tk("agent", "ls", "-json", "-limit", "private-invalid-integer", okay=False)
            assert not invalid.stdout and "private-invalid-integer" not in invalid.stderr

            text = "CLI capture fixture\n\nKeep Unicode: 月 🌙\n  and exact indentation.\n"
            source = folder / "notes.txt"
            source.write_bytes(text.encode())
            common = ["-project", "Flight", "-title", "CLI capture fixture", "-request-id", "same-capture", "-json"]
            file_result = json.loads(tk("import", str(source), *common).stdout)
            pipe_result = json.loads(tk("import", "-i", *common, input=text).stdout)
            assert file_result["id"] == pipe_result["id"] and file_result["text"] == pipe_result["text"] == text
            assert len(receipts) == 1
            assert receipts["same-capture"][0]["origin"] == "import"

            if os.name == "posix":
                import pty
                master, slave = pty.openpty()
                process = None
                try:
                    process = subprocess.Popen([str(args.binary), "import", "-i", "-project", "Flight", "-request-id", "terminal-capture"], env=environment, stdin=slave, stderr=slave, stdout=subprocess.PIPE)
                    os.close(slave)
                    slave = -1
                    prompt = b""
                    deadline = time.monotonic() + 3
                    while b"Ctrl-D" not in prompt:
                        remaining = deadline - time.monotonic()
                        if remaining <= 0 or not select.select([master], [], [], remaining)[0]:
                            raise AssertionError("Multiline terminal prompt was not presented")
                        prompt += os.read(master, 4096)
                    os.write(master, text.encode() + b"\x04")
                    output, _ = process.communicate(timeout=3)
                    assert process.returncode == 0 and b"Classification follows separately" in output
                    assert receipts["terminal-capture"][0]["text"] == text
                    assert receipts["terminal-capture"][0]["projectId"] == 2
                finally:
                    if process and process.poll() is None:
                        process.terminate()
                        process.wait(timeout=2)
                    os.close(master)
                    if slave >= 0:
                        os.close(slave)
            # The terminal must never echo the password; redirected output remains JSON-safe.
            login_env = dict(environment, TELEPORT_CONFIG=str(folder / "login-client.json"))
            login_env.pop("TELEPORT_USERNAME", None)
            login_env.pop("TELEPORT_PASSWORD", None)
            if os.name == "posix":
                master, slave = pty.openpty()
                process = subprocess.Popen([str(args.binary), "login", "-json"], env=login_env, stdin=slave, stderr=slave, stdout=subprocess.PIPE)
                os.close(slave)
                transcript = b""
                try:
                    def await_prompt(expected):
                        nonlocal transcript
                        deadline = time.monotonic() + 3
                        while expected not in transcript:
                            remaining = deadline - time.monotonic()
                            if remaining <= 0 or not select.select([master], [], [], remaining)[0]:
                                raise AssertionError("Login prompt was not presented")
                            transcript += os.read(master, 4096)
                    await_prompt(b"Username:")
                    os.write(master, b"pilot\n")
                    await_prompt(b"Password:")
                    os.write(master, b"fixture-secret\n")
                    stdout, _ = process.communicate(timeout=3)
                    while select.select([master], [], [], 0)[0]:
                        try:
                            chunk = os.read(master, 4096)
                            if not chunk:
                                break
                            transcript += chunk
                        except OSError:
                            break
                    assert process.returncode == 0 and json.loads(stdout)["username"] == "pilot"
                    assert b"fixture-secret" not in transcript and b"fixture-secret" not in stdout
                finally:
                    if process.poll() is None:
                        process.terminate()
                        process.wait(timeout=2)
                    os.close(master)
            env_login = dict(login_env, TELEPORT_USERNAME="pilot", TELEPORT_PASSWORD="fixture-secret")
            result = subprocess.run([str(args.binary), "login", "-json"], env=env_login, input="", capture_output=True, text=True, timeout=5)
            assert result.returncode == 0 and json.loads(result.stdout)["username"] == "pilot"
            assert "fixture-secret" not in result.stdout + result.stderr
            missing = subprocess.run([str(args.binary), "login"], env=login_env, input="", capture_output=True, text=True, timeout=5)
            assert missing.returncode != 0 and "TELEPORT_USERNAME" in missing.stderr
            old_server[0] = True
            tk("import", str(source), "-project", "Flight", "-request-id", "legacy-server")
            assert "origin" not in receipts["legacy-server"][0]
            assert not failures, failures
    finally:
        fixture.shutdown()
        fixture.server_close()
        thread.join(timeout=2)
    print("PASS: real binary catalogue, noun/action dispatch, typed filters, native/API separation, file and piped capture, stable retries and terminal multiline EOF input; zero database/browser/model calls")


if __name__ == "__main__":
    from tool_history import run
    run(main)
