"""Validate the standalone specification without rewriting it.

Usage: python3 tools/standalone_spec.py [SPECIFICATION.md]
This compatibility entry point delegates to the current documentation checker.
"""

import argparse
from pathlib import Path

from spec_sources import verify


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("document", nargs="?", type=Path,
                        default=Path(__file__).resolve().parents[1] / "SPECIFICATION.md")
    args = parser.parse_args()
    document = args.document.resolve()
    verify(document.parent, document)


if __name__ == "__main__":
    from tool_history import run
    run(main)
