"""Recover completed file changes from one explicitly selected Codex session log.

Only recorded file changes are replayed; stored shell commands are never run.
Use --report first, then supply --since and --apply for the selected phase.
"""
import argparse
import json
from pathlib import Path
import re
import subprocess


def apply_diff(content, diff):
    lines = content.splitlines(keepends=True)
    hunks = []
    for line in diff.splitlines(keepends=True):
        if line.startswith("@@"):
            match = re.match(r"@@ -(\d+)(?:,\d+)? \+\d+(?:,\d+)? @@", line)
            if not match:
                raise ValueError("Invalid hunk header")
            hunks.append((int(match[1]) - 1, [], []))
        elif hunks and line[:1] in (" ", "+", "-"):
            if line[0] != "+":
                hunks[-1][1].append(line[1:])
            if line[0] != "-":
                hunks[-1][2].append(line[1:])
        elif line.startswith("\\ No newline"):
            for side in hunks[-1][1:]:
                if side:
                    side[-1] = side[-1].rstrip("\n")
    offset = 0
    for start, old, new in hunks:
        expected = max(0, start + offset)
        if lines[expected:expected + len(old)] == old:
            position = expected
        else:
            positions = [i for i in range(len(lines) - len(old) + 1) if lines[i:i + len(old)] == old]
            if len(positions) != 1:
                raise ValueError(f"Hunk has {len(positions)} exact matches at line {start + 1}")
            position = positions[0]
        lines[position:position + len(old)] = new
        offset += len(new) - len(old)
    return "".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("log", type=Path)
    parser.add_argument("--source-root", required=True)
    parser.add_argument("--target-root", type=Path, required=True)
    parser.add_argument("--since", default="")
    parser.add_argument("--report", action="store_true")
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    root = args.target_root.resolve()
    source = args.source_root.rstrip("/") + "/"
    count = 0
    for number, line in enumerate(args.log.open(), 1):
        record = json.loads(line)
        payload = record.get("payload", {})
        command = payload.get("input", "")
        if args.report and isinstance(command, str) and "git switch -c phase/07-08-native-harness" in command:
            print("PHASE", record.get("timestamp"), "record", number)
        item = payload.get("item", {})
        if payload.get("type") != "item_completed" or item.get("type") != "FileChange" or item.get("status") != "completed" or record.get("timestamp", "") < args.since:
            continue
        changes = {path: value for path, value in item.get("changes", {}).items() if path.startswith(source)}
        if not changes:
            continue
        if args.report:
            print(record.get("timestamp"), number, ", ".join(path[len(source):] for path in changes))
        if not args.apply:
            continue
        for old_path, change in changes.items():
            path = root / old_path[len(source):]
            if not path.resolve().is_relative_to(root) or change.get("move_path"):
                raise ValueError("Refuse path escape or unreviewed move")
            if change["type"] == "add":
                if path.exists():
                    raise ValueError(f"Refuse to overwrite added file: {path}")
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(change["content"])
            elif change["type"] == "update":
                try:
                    result = apply_diff(path.read_text(), change["unified_diff"])
                except ValueError:
                    if path.suffix != ".go":
                        raise
                    subprocess.run(["gofmt", "-w", str(path)], check=True)
                    result = apply_diff(path.read_text(), change["unified_diff"])
                path.write_text(result)
            else:
                raise ValueError(f"Refuse change type {change['type']}")
            count += 1
    print(f"Recovered {count} file changes" if args.apply else "Report only; no changes applied")


if __name__ == "__main__":
    main()
