"""Build tk once with traceable local metadata; optionally install that artifact."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--install-only", action="store_true")
    args = parser.parse_args()
    binary = ROOT / "bin" / "tk"
    if not args.install_only:
        counter = ROOT / ".build-count"
        count = int(counter.read_text()) + 1 if counter.exists() else 1
        revision = subprocess.check_output(["git", "rev-parse", "--short", "HEAD"], cwd=ROOT, text=True).strip()
        build_version = os.environ.get("TELEPORT_BUILD_VERSION", f"0.1.0-dev.{count}+{revision}.dirty")
        binary.parent.mkdir(exist_ok=True)
        subprocess.run(["go", "build", "-trimpath", "-ldflags", f"-X main.version={build_version}", "-o", str(binary), "./cmd/tk"], cwd=ROOT, check=True)
        counter.write_text(str(count))
        print(f"Built {binary.relative_to(ROOT)} ({build_version})")
    else:
        env = subprocess.check_output(["go", "env", "GOBIN", "GOPATH"], text=True).splitlines()
        destination = Path(env[0] or str(Path(env[1].split(os.pathsep)[0]) / "bin"))
        destination.mkdir(parents=True, exist_ok=True)
        shutil.copy2(binary, destination / "tk")
        print(f"Installed {destination / 'tk'}")

if __name__ == "__main__":
    main()
