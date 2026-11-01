"""Install the pinned acceptance browser client into an isolated reusable cache."""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys

from tool_history import locked_history

ROOT = Path(__file__).resolve().parents[1]
REQUIREMENTS = ROOT / "tools/browser_requirements.txt"


def environment_path():
    identity = hashlib.sha256(REQUIREMENTS.read_bytes() + sys.version.encode() + sys.platform.encode()).hexdigest()[:16]
    return ROOT / ".cache/tools" / f"browser-{identity}"


def browser_python():
    folder = environment_path()
    executable = folder / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
    if not executable.is_file() or not (folder / ".ready").is_file() or not (folder / ".browser-ready").is_file():
        raise SystemExit("Browser client is not prepared. Run: make browser-tools")
    return str(executable)


def runtime_for_args(arguments):
    if "--browser" not in arguments or "--help" in arguments or "-h" in arguments:
        return None
    if "--browser-driver=cli" in arguments or any(arguments[i:i+2] == ["--browser-driver", "cli"] for i in range(len(arguments))):
        return None
    return browser_python()


def main():
    folder = environment_path()
    with locked_history(folder.parent / "browser-setup"):
        if not (folder / ".ready").is_file():
            if folder.exists():
                shutil.rmtree(folder)
            subprocess.run([sys.executable, "-m", "venv", str(folder)], check=True, timeout=60)
            executable = folder / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
            uv = shutil.which("uv")
            command = [uv, "pip", "install", "--python", str(executable)] if uv else [str(executable), "-m", "pip", "install"]
            subprocess.run([*command, "-r", str(REQUIREMENTS)], check=True, timeout=120)
            (folder / ".ready").write_text(REQUIREMENTS.read_text())
        if not (folder / ".browser-ready").is_file():
            executable = folder / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
            subprocess.run([str(executable), "-m", "playwright", "install", "chromium", "--only-shell"], check=True, timeout=120)
            (folder / ".browser-ready").write_text("ready")
    print(f"PASS: pinned browser client ready at {browser_python()}")


if __name__ == "__main__":
    from tool_history import run
    run(main)
