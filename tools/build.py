"""Build tk once with traceable local metadata; optionally install that artifact."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from source_cache import source_identity, verified_artifact, record_artifact
from tool_history import locked_history

ROOT = Path(__file__).resolve().parents[1]


def install_artifact(binary, destination):
    """Replace the directory entry atomically so running executables retain their bytes."""
    destination = Path(destination)
    destination.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=".tk-install-", dir=destination.parent)
    os.close(descriptor)
    temporary = Path(temporary)
    try:
        shutil.copy2(binary, temporary)
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)

def local_version(root, count):
    revision = subprocess.check_output(["git", "rev-parse", "--short", "HEAD"], cwd=root, text=True).strip()
    dirty = subprocess.check_output(["git", "status", "--porcelain=v1", "--untracked-files=normal"], cwd=root, text=True).strip()
    return f"0.1.0-dev.{count}+{revision}" + (".dirty" if dirty else "")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--install-only", action="store_true")
    parser.add_argument("--reuse", action="store_true", help="Reuse a verified binary with identical inputs and source identity")
    parser.add_argument("--debug", action="store_true", help="Retain native debug symbols and use ordinary C optimization")
    args = parser.parse_args()
    binary = ROOT / "bin" / "tk"
    manifest = ROOT / ".cache/tools/build.json"
    with locked_history(manifest):
        build(args, binary, manifest)


def build(args, binary, manifest):
    if not args.install_only:
        subprocess.run([sys.executable, "-B", str(ROOT / "tools/pack_web.py")], cwd=ROOT, check=True)
        identity = source_identity(ROOT, "build-v3-packed-debug" if args.debug else "build-v3-packed-size-optimized")
        # Metadata changes on commits or dirty transitions even if Go inputs do not.
        identity = identity + local_version(ROOT, 0) if identity else None
        if args.reuse and verified_artifact(manifest, identity, binary):
            print("PASS: reused verified bin/tk (source, toolchain, target and flags unchanged)")
            return
        counter = ROOT / ".build-count"
        count = int(counter.read_text()) + 1 if counter.exists() else 1
        build_version = os.environ.get("TELEPORT_BUILD_VERSION") or local_version(ROOT, count)
        binary.parent.mkdir(exist_ok=True)
        environment = dict(os.environ)
        environment.setdefault("CGO_CFLAGS", "-O2 -g" if args.debug else "-Os -g0")
        linker_flags = f"-X main.version={build_version}" if args.debug else f"-s -w -X main.version={build_version}"
        subprocess.run(["go", "build", "-trimpath", "-ldflags", linker_flags, "-o", str(binary), "./cmd/tk"], cwd=ROOT, env=environment, check=True)
        counter.write_text(str(count))
        record_artifact(manifest, identity, binary)
        print(f"Built {binary.relative_to(ROOT)} ({build_version})")
    else:
        env = subprocess.check_output(["go", "env", "GOBIN", "GOPATH"], text=True).splitlines()
        destination = Path(env[0] or str(Path(env[1].split(os.pathsep)[0]) / "bin"))
        destination.mkdir(parents=True, exist_ok=True)
        install_artifact(binary, destination / "tk")
        print(f"Installed {destination / 'tk'}")

if __name__ == "__main__":
    from tool_history import run
    run(main)
