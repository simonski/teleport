"""Normalize/check documentation for the supported CLI and browser clients.

Native harness terminals remain supported. The removed application terminal
client must not reappear in canonical product or delivery documentation.
"""
import argparse
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
DOCUMENTS = ("SPECIFICATION.md", "IMPLEMENTATION_PLAN.md", "API_CONTRACT.md", "USER_GUIDE.md")


def normalize(text):
    lines = []
    for line in text.splitlines(keepends=True):
        if line.startswith("Bubble Tea TUI is an API client.") or line.startswith("Honor `TELEPORT_NO_ANIMATION`"):
            continue
        if line.startswith("| Remove application TUI |"):
            line = "| Client scope | CLI and browser are the application clients; native harness terminals remain supported. | Remove the application terminal client from target requirements/authentication/release scope; no such implementation needs deletion. |\n"
        for old, new in (
            ("[11. CLI and TUI](#11-cli-and-tui)", "[11. CLI](#11-cli)"),
            ("## 11. CLI and TUI", "## 11. CLI"),
            ("CLI, TUI, browser", "CLI, browser"),
            ("CLI / TUI / browser", "CLI / browser"),
            ("CLI/TUI", "CLI"),
            ("`-g`/gui/tui", "`-g`/gui"),
            (", with Make development entry points and Bubble Tea for TUI", ", with Make development entry points"),
            ("Go, Make, Bubble Tea, and", "Go, Make, and"),
            ("web, CLI, TUI where present, scheduler", "web, CLI, scheduler"),
            ("full TUI/PTY/console", "PTY/console"),
            ("application TUI, ", ""),
            ("external authentication, application TUI", "external authentication"),
            ("clients, TUI, isolated", "clients, isolated"),
            ("engine/TUI", "engine/native-terminal"),
            ("native TUI", "native terminal"),
            ("actual TUI", "native terminal"),
            ("direct-TUI", "direct-terminal"),
            ("TUI scope", "client scope"),
        ):
            line = line.replace(old, new)
        lines.append(line)
    return "".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="Apply the explicit client-scope documentation migration")
    args = parser.parse_args()
    for name in DOCUMENTS:
        path = ROOT / name
        text = path.read_text()
        if args.write:
            text = normalize(text)
            path.write_text(text)
        if re.search(r"\bTUI\b|\btui\b|Bubble Tea", text):
            raise SystemExit(f"Unsupported application client reference in {name}")
    print("PASS: canonical documentation describes CLI/browser clients and native harness terminals")


if __name__ == "__main__":
    main()
