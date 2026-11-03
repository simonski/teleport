"""Transfer offline state through an archived init bridge, retaining verified rollback data."""
import argparse
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import select
import secrets
import subprocess
import tempfile

@contextmanager
def migration(legacy, binary, source, config, archive, on_abort=lambda:None):
    for path in (source, config):
        if path.is_symlink() or not path.is_file():
            raise RuntimeError("Migration inputs must be regular files")
    archive.mkdir(mode=0o700, parents=True, exist_ok=True)
    if archive.is_symlink() or archive.resolve() != archive:
        raise RuntimeError("Migration archive must not traverse symlinks")
    archive.chmod(0o700)
    legacy_environment = dict(os.environ, TELEPORT_DB=str(source))
    target_environment = dict(os.environ)
    target_environment.pop("TELEPORT_DB", None)
    with tempfile.TemporaryFile(mode="w+b") as errors:
        process = subprocess.Popen([str(legacy), "init", "-export-state-stdout"], env=legacy_environment, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errors)
        moved=[]
        try:
            if not select.select([process.stdout], [], [], 30)[0]:
                raise RuntimeError("Historical export did not produce a snapshot")
            body = process.stdout.readline((256 << 20) + 2)
            if not body or len(body) > 256 << 20:
                raise RuntimeError("Historical export failed or exceeded the snapshot limit")
            state = json.loads(body)
            identity = state["installationId"]
            digest = hashlib.sha256(body).hexdigest()
            snapshot = archive / (digest + ".json")
            if not snapshot.exists():
                with open(snapshot,"xb", opener=lambda path, flags: os.open(path, flags, 0o600)) as output:
                    output.write(body); output.flush(); os.fsync(output.fileno())
            errors.seek(0)
            line = errors.read().decode().strip()
            if not line.startswith("Backup: "):
                raise RuntimeError("Historical export did not confirm a verified backup")
            backup = Path(line.removeprefix("Backup: "))
            if backup.is_symlink() or backup.parent != source.parent or not backup.is_file():
                raise RuntimeError("Historical backup identity is invalid")
            archived_backup=archive/(digest+"-"+secrets.token_hex(8)+".backup")
            backup.replace(archived_backup)
            archived_backup.chmod(0o600)
            command = [str(binary), "init", "-db-config", str(config)]
            result = subprocess.run([*command, "-restore-state-stdin", "-state-sha256", digest], input=body, env=target_environment, capture_output=True, timeout=30)
            if result.returncode:
                raise RuntimeError("Snapshot restore refused: " + result.stderr.decode().strip())
            check = subprocess.run([*command, "-export-state-stdout"], env=target_environment, capture_output=True, timeout=30)
            if check.returncode or hashlib.sha256(check.stdout).hexdigest() != digest:
                raise RuntimeError("PostgreSQL read-back did not match the original snapshot")
            from deploy_remote import secure_json
            secure_json(archive/"verification.json",{"installationId":identity,"sha256":digest,"verified":True})
            if process.poll() is not None:
                raise RuntimeError("Historical ownership ended before cutover")
            for suffix in ("","-wal","-shm"):
                original=Path(str(source)+suffix)
                if original.exists():
                    destination=archive/("source.database"+suffix)
                    original.replace(destination);moved.append((original,destination))
            # The caller retains old-store ownership through service health and
            # archival. Releasing earlier permits two independently writable stores.
            yield {"installationId": identity, "sha256": digest, "ownerPid": process.pid}
            process.stdin.write(b"\n"); process.stdin.flush()
            if process.wait(timeout=5):
                raise RuntimeError("Historical export did not release cleanly")
        except BaseException:
            on_abort()
            for original,destination in reversed(moved):destination.replace(original)
            raise
        finally:
            if process.poll() is None:
                process.terminate()
                try: process.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    process.kill(); process.wait(timeout=3)
            for pipe in (process.stdin,process.stdout):
                try:pipe.close()
                except BrokenPipeError:pass


def transfer(legacy, binary, source, config, archive):
    """Perform an offline cutover with no service start; source is retained in archive."""
    with migration(legacy, binary, source, config, archive) as verified:
        pass
    print(f"PASS: offline installation cutover {verified['installationId']}; private rollback archive: {archive}",flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="action", required=True)
    migrate = commands.add_parser("transfer")
    for name in ("legacy", "binary", "source", "config", "archive"):
        migrate.add_argument("--" + name, type=Path, required=True)
    args = parser.parse_args()
    transfer(args.legacy.absolute(), args.binary.absolute(), args.source.absolute(), args.config.absolute(), args.archive.absolute())


if __name__ == "__main__":
    from tool_history import run
    run(main)
