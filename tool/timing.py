#!/usr/bin/env python3
"""Print each tool invocation and its duration in seconds from the CSV history."""
import argparse
import csv
import math
import os
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))
from tool_history import read_history, run


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("history", nargs="?", type=Path,
                        default=Path(os.environ.get("TELEPORT_TOOL_HISTORY", ROOT / "tool_calls_history.csv")))
    args = parser.parse_args()
    try:
        fields, rows = read_history(args.history)
    except (OSError, csv.Error) as error:
        parser.exit(1, f"timing: {error}\n")
    required = {"tool", "started_at", "outcome", "exit_code", "duration_seconds"}
    if not required.issubset(fields):
        parser.error("history CSV requires columns: " + ", ".join(sorted(required)))
    lines = []
    for number, row in enumerate(rows, 2):
        try:
            seconds = float(row["duration_seconds"])
            if None in row or not math.isfinite(seconds) or seconds < 0 or any(row[field] is None for field in required):
                raise ValueError
        except (TypeError, ValueError):
            parser.error(f"invalid history record at CSV line {number}")
        lines.append(f"{row['started_at']}  {row['tool']}  {row['outcome']}  exit={row['exit_code'] or '-'}  {seconds:.6f} seconds")
    if lines:
        print("\n".join(lines))


if __name__ == "__main__":
    run(main)
