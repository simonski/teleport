"""Check source JavaScript syntax and vendored browser integrity without building assets."""
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def main():
    node = shutil.which("node")
    if not node:
        raise SystemExit("Node.js is required for development JavaScript checks (not for running tk)")
    sources = sorted((ROOT / "internal/server/web").glob("*.js"))
    subprocess.run([node, "--experimental-vm-modules", "--no-warnings", str(ROOT / "tools/web_syntax.cjs"), *map(str, sources)], check=True)
    tests = sorted((ROOT / "internal/server/webtests").glob("*_test.cjs"))
    subprocess.run([node, "--test", *map(str, tests)], check=True)
    subprocess.run(["python3", "-B", str(ROOT / "tools/vendor_web.py"), "--verify"], check=True)
    subprocess.run(["python3", "-B", str(ROOT / "tools/vendor_terminal.py"), "--verify"], check=True)
    print(f"PASS: {len(sources)} browser source syntax checks")


if __name__ == "__main__":
    from tool_history import run
    run(main)
