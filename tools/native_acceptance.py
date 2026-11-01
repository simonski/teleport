"""Run one explicitly authorized real Codex turn in an isolated installation.

Never included in routine checks. Native configuration/permissions are retained;
startup prompts are not automatically answered. No human API credential is
passed to Codex. Failed/unresolved process ownership retains private evidence.
"""
import argparse
import base64
import hashlib
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
MARKER = "TK_NATIVE_ACCEPTANCE_OK"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--authorized", action="store_true", help="Separate user authorization for this real provider acceptance run has been obtained")
    args = parser.parse_args()
    if not args.authorized:
        parser.error("Real provider acceptance requires separate explicit authorization")
    folder = Path(tempfile.mkdtemp(prefix="teleport-native-acceptance-"))
    os.chmod(folder, 0o700)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    environment = dict(os.environ, TELEPORT_DB=str(folder / "database.db"), TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
    installed = shutil.which("codex")
    if not installed:
        raise RuntimeError("The native Codex executable is unavailable")
    fixture_bin = folder / "bin"
    fixture_bin.mkdir(mode=0o700)
    (fixture_bin / "codex-original").symlink_to(Path(installed).resolve())
    shutil.copyfile(ROOT / "tools/native_trust_wrapper.py", fixture_bin / "codex")
    (fixture_bin / "codex").chmod(0o500)
    environment["PATH"] = str(fixture_bin) + os.pathsep + environment["PATH"]
    native_config_root = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex")))
    protected = [native_config_root / "config.toml", *sorted((native_config_root / "rules").glob("*.rules"))]
    hashes = {path: hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None for path in protected}
    process = None
    identity = None
    clean = False
    succeeded = False
    log = (folder / "server.log").open("w")

    def tk(operation, data=None):
        command = [str(ROOT / "bin/tk"), operation]
        if data is not None:
            command += ["--data", json.dumps(data)]
        result = subprocess.run(command, env=environment, text=True, capture_output=True, timeout=20)
        if result.returncode:
            raise RuntimeError(result.stderr.strip())
        return json.loads(result.stdout)

    def wait_for(callback, timeout=30):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            value = callback()
            if value:
                return value
            time.sleep(0.1)
        raise AssertionError("Native acceptance observation timed out")

    def ready():
        try:
            with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                return response.status == 200
        except OSError:
            return False

    try:
        subprocess.run([str(ROOT / "bin/tk"), "init"], env=environment, check=True, capture_output=True, timeout=20)
        process = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}", "--runtime-dir", str(folder / "runtime")], env=environment, stdout=log, stderr=log)
        wait_for(ready)
        subprocess.run([str(ROOT / "bin/tk"), "login", "--password-stdin"], env=environment, input="password\n", text=True, check=True, capture_output=True, timeout=20)
        repository = folder / "repository"
        repository.mkdir()
        for command in (["init", "-b", "main"],):
            subprocess.run(["git", "-C", str(repository), *command], check=True, capture_output=True, timeout=20)
        (repository / "fixture.txt").write_text("Owned native acceptance fixture.\n")
        for command in (["add", "fixture.txt"], ["commit", "-m", "Seed isolated native fixture"]):
            subprocess.run(["git", "-c", f"core.hooksPath={os.devnull}", "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "-C", str(repository), *command], check=True, capture_output=True, timeout=20)
        project = tk("createProject", {"name": "Native acceptance", "key": "native-acceptance", "requestId": "native-project"})
        project = tk("configureLocalRepository", {"projectId": project["id"], "expectedVersion": project["version"], "repository": str(repository), "storageRoot": str(folder / "worktrees")})
        row = tk("openConsoleSession", {"projectId": project["id"], "harness": "codex", "requestId": "native-session"})
        identity = row["id"]

        def connected():
            manifest = json.loads((folder / "runtime/sessions.json").read_text())
            current = manifest[identity]
            output = tk("attachSession", {"id": identity})
            (folder / "terminal.txt").write_bytes(base64.b64decode(output.get("data", "")))
            if current["state"] in ("failed", "exited", "unknown"):
                raise RuntimeError("Native startup did not retain a verified live process; private terminal evidence retained")
            return current.get("nativeThreadId")

        wait_for(connected)
        prompt = "Reply exactly " + MARKER + ". Do not modify files or native configuration."
        receipt = tk("sendSessionInput", {"id": identity, "requestId": "authorized-native-turn", "data": base64.b64encode((prompt + "\r").encode()).decode()})
        assert receipt["state"] == "delivered", receipt

        def finished():
            evidence = tk("getSessionTurn", {"id": identity})
            turn = evidence.get("turn")
            output = tk("attachSession", {"id": identity})
            (folder / "terminal.txt").write_bytes(base64.b64decode(output.get("data", "")))
            if evidence.get("failure"):
                raise RuntimeError("Native acceptance failed with " + evidence["failure"]["code"] + "; no replay was started")
            if not turn or turn["state"] == "inProgress":
                return None
            return turn

        turn = wait_for(finished, timeout=180)
        assert turn["state"] == "completed" and not turn["truncated"] and turn["text"].strip() == MARKER, "Native turn did not produce the exact successful structured acceptance result"
        for path, before in hashes.items():
            after = hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None
            assert before == after, "Native configuration/rules changed; no automatic rollback was attempted"
        tk("stopSession", {"id": identity, "confirm": True})
        wait_for(lambda: all(row["state"] in ("exited", "failed") for row in tk("listSessions")["items"]))
        tk("shutdownLocalRuntime", {"confirm": True})
        clean = True
        succeeded = True
        print("PASS: authorized real Codex native PTY/thread, one completed provider turn, exact final result, unchanged native configuration/rules and observed owned exit")
    finally:
        if not clean and process is not None and process.poll() is None:
            try:
                if identity:
                    tk("stopSession", {"id": identity, "confirm": True})
                    wait_for(lambda: all(row["state"] in ("exited", "failed") for row in tk("listSessions")["items"]))
                tk("shutdownLocalRuntime", {"confirm": True})
                clean = True
            except (RuntimeError, AssertionError):
                pass
        if process is not None and process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
        log.close()
        if clean and identity:
            print(f"Private native acceptance evidence: {folder}")
        elif clean and succeeded:
            shutil.rmtree(folder)
        else:
            print(f"Retain unresolved native acceptance evidence: {folder}")


if __name__ == "__main__":
    main()
