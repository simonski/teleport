#!/usr/bin/env python3
"""Acceptance-only launcher: trust the owned fixture without native config edits.

Copy beside a codex-original symlink inside an owned acceptance fixture bin/.
OS command sandbox/approval rules are inherited without overrides.
"""
import json
import os
from pathlib import Path
import sys


def main():
    binary_folder = Path(__file__).absolute().parent
    fixture = binary_folder.parent
    if not fixture.name.startswith("teleport-native-acceptance-"):
        raise SystemExit("An explicitly owned native acceptance fixture is required")
    native = (binary_folder / "codex-original").resolve(strict=True)
    arguments = sys.argv[1:]
    if arguments == ["--version"]:
        os.execv(str(native), [str(native), *arguments])
    working = Path.cwd().resolve()
    if not working.is_relative_to((fixture / "worktrees").resolve()):
        raise SystemExit("Native acceptance launch must stay in its owned worktree")
    trusted = {str(working), str(fixture / "repository"), str((fixture / "repository").resolve())}
    # The native CLI's dotted-key override grammar does not parse quoted path
    # segments as TOML keys. Supply the owned map as one TOML inline table.
    table = ",".join(json.dumps(path) + '={trust_level="trusted"}' for path in sorted(trusted))
    overrides = ["--config", "projects={" + table + "}"]
    os.execv(str(native), [str(native), *overrides, *arguments])


if __name__ == "__main__":
    main()
