"""Inspect the installed Codex protocol without starting a session or inference.

The report proves local schema/argv compatibility only. It never reads credentials,
changes user rules/configuration, starts an agent turn or authorizes managed execution.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--export-schemas", type=Path, help="Copy generated schemas to a new owned directory")
    args = parser.parse_args()
    executable = shutil.which(args.codex)
    if not executable:
        parser.error("Codex is not installed; no capability is verified")
    def run(*arguments):
        return subprocess.run([executable, *arguments], capture_output=True, text=True, check=True, timeout=30).stdout
    version = run("--version").strip()
    with tempfile.TemporaryDirectory(prefix="teleport-codex-schema-") as temporary:
        run("app-server", "generate-json-schema", "--experimental", "--out", temporary)
        paths = sorted(Path(temporary).rglob("*.json"))
        if args.export_schemas:
            shutil.copytree(temporary, args.export_schemas)
        schemas = {}
        digest = hashlib.sha256()
        methods = set()
        def walk(value):
            if isinstance(value, dict):
                method = value.get("properties", {}).get("method", {})
                if "const" in method:
                    methods.add(method["const"])
                methods.update(method.get("enum", []))
                for child in value.values():
                    walk(child)
            elif isinstance(value, list):
                for child in value:
                    walk(child)
        for path in paths:
            content = path.read_bytes()
            digest.update(str(path.relative_to(temporary)).encode() + b"\0" + content)
            value = json.loads(content)
            schemas[str(path.relative_to(temporary))] = value
            walk(value)
        interesting = {name: schema for name, schema in schemas.items() if any(key in name for key in ["ThreadStartParams", "AskForApproval", "CommandExecutionRequestApproval", "ExecServer", "Environment"])}
        report = {
            "version": version, "schemaSha256": digest.hexdigest(), "schemaFiles": len(paths),
            "methods": sorted(methods), "inspectedSchemas": interesting,
            "verified": ["installed version", "local experimental JSON Schema generation"],
            "managedExecutionEnabled": False,
            "unverified": ["central per-command enforcement for future centrally enforced mode (not required by trusted native-local mode)", "native Codex owned process and command recovery", "safe tool-result continuation without uncertain replay", "read-only Review filesystem enforcement", "live provider acceptance (separate authorization required)"],
        }
    rendered = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(rendered)
        print(f"PASS: {version}; {len(paths)} local schemas; report {args.output}")
    else:
        print(rendered)


if __name__ == "__main__":
    main()
