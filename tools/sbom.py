"""Generate a CycloneDX SBOM for tk and its bundled browser dependencies."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
from urllib.parse import quote
from source_cache import source_identity, verified_artifact, record_artifact
from tool_history import locked_history

from vendor_web import DESTINATION as VENDOR, verify as verify_web
from vendor_terminal import PACKAGES, verify as verify_terminal

ROOT = Path(__file__).resolve().parents[1]
GENERATOR = "github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.12.0"


def browser_components():
    """Use verified local bytes, retaining upstream archive integrity separately."""
    verify_web()
    verify_terminal()
    web = json.loads((VENDOR / "manifest.json").read_text())
    terminal = json.loads((VENDOR / "terminal/manifest.json").read_text())
    packages = [(web["package"], web["version"], web, VENDOR,
                 list(web["sha256"]), "LICENSE")]
    for name, version, files in PACKAGES:
        metadata = next(p for p in terminal["packages"] if p["name"] == name)
        packages.append((name, version, metadata, VENDOR / "terminal",
                         list(files.values()), next(f for f in files.values() if f.endswith("LICENSE"))))
    components = []
    for name, version, metadata, directory, files, license_file in packages:
        purl = f"pkg:npm/{quote(name, safe='/')}@{version}"
        algorithm, integrity = metadata["integrity"].split("-", 1)
        if algorithm != "sha512":
            raise ValueError(f"Unsupported archive integrity for {name}: {algorithm}")
        component = {
            "type": "library", "bom-ref": purl, "name": name,
            "version": version, "purl": purl,
            "licenses": [{"license": {"id": "MIT", "text": {
                "contentType": "text/plain", "content": (directory / license_file).read_text(),
            }}}],
            "externalReferences": [{"type": "distribution", "url": metadata["source"],
                "hashes": [{"alg": "SHA-512", "content": base64.b64decode(integrity, validate=True).hex()}]}],
            "components": [],
        }
        if metadata.get("patch"):
            component["properties"] = [{"name": "teleport:vendor:patch", "value": metadata["patch"]}]
        for filename in sorted(files):
            path = directory / filename
            relative = path.relative_to(ROOT).as_posix()
            component["components"].append({
                "type": "file", "bom-ref": f"{purl}#{filename}", "name": relative,
                "hashes": [{"alg": "SHA-256", "content": hashlib.sha256(path.read_bytes()).hexdigest()}],
            })
        components.append(component)
    return components


def add_browser_components(bom):
    components = browser_components()
    bom.setdefault("components", []).extend(components)
    root = bom["metadata"]["component"]["bom-ref"]
    dependencies = bom.setdefault("dependencies", [])
    root_dependency = next((d for d in dependencies if d["ref"] == root), None)
    if root_dependency is None:
        root_dependency = {"ref": root, "dependsOn": []}
        dependencies.append(root_dependency)
    root_dependency.setdefault("dependsOn", []).extend(c["bom-ref"] for c in components)
    for component in components:
        dependencies.append({"ref": component["bom-ref"], "dependsOn": [c["bom-ref"] for c in component["components"]]})
        dependencies.extend({"ref": c["bom-ref"], "dependsOn": []} for c in component["components"])
    return bom


def add_runtime_license(bom):
    """The generator does not currently detect the standard library's license."""
    goroot = Path(subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, text=True).strip())
    license_text = (goroot / "LICENSE").read_text()
    for component in bom.get("components", []):
        if component.get("name") == "std":
            component["licenses"] = [{"license": {"id": "BSD-3-Clause", "text": {
                "contentType": "text/plain", "content": license_text,
            }}}]
    return bom


def add_source_identity(bom):
    revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    dirty = bool(subprocess.check_output(["git", "status", "--porcelain=v1", "--untracked-files=normal"],
                                        cwd=ROOT, text=True).strip())
    bom["metadata"]["component"].setdefault("properties", []).extend([
        {"name": "teleport:source:revision", "value": revision},
        {"name": "teleport:source:dirty", "value": str(dirty).lower()},
    ])
    return bom


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "sbom.cdx.json")
    parser.add_argument("--refresh", action="store_true", help="Regenerate the Go inventory even when verified inputs match")
    args = parser.parse_args()
    identity = source_identity(ROOT, "sbom-v1/" + GENERATOR)
    cache = ROOT / ".cache/tools/go-sbom.json"
    manifest = cache.with_suffix(".manifest.json")
    with locked_history(manifest):
        if args.refresh or not verified_artifact(manifest, identity, cache):
            with tempfile.TemporaryDirectory(prefix="teleport-sbom-") as temporary:
                output = Path(temporary) / "go.cdx.json"
                subprocess.run(["go", "run", GENERATOR, "app", "-json", "-std", "-licenses",
                        "-output-version", "1.6", "-main", "cmd/tk", "-output", str(output), str(ROOT)],
                       cwd=ROOT, check=True)
                cache.parent.mkdir(parents=True, exist_ok=True)
                staged_cache = cache.with_suffix(".tmp")
                staged_cache.write_bytes(output.read_bytes())
                staged_cache.replace(cache)
                record_artifact(manifest, identity, cache)
        # Browser bytes and licenses are always reverified. Source metadata is
        # regenerated instead of returning a stale previously published SBOM.
        bom = add_source_identity(add_runtime_license(add_browser_components(json.loads(cache.read_text()))))
    from datetime import datetime, timezone
    import uuid
    bom["serialNumber"] = "urn:uuid:" + str(uuid.uuid4())
    bom["metadata"]["timestamp"] = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    # Publish only a complete document, preserving an existing SBOM on failure.
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=args.output.parent, delete=False) as staged:
        temporary_output = Path(staged.name)
        try:
            staged.write(json.dumps(bom, indent=2) + "\n")
            staged.close()
            temporary_output.replace(args.output)
        finally:
            temporary_output.unlink(missing_ok=True)
    print(f"PASS: CycloneDX {bom['specVersion']} SBOM written to {args.output}")


if __name__ == "__main__":
    from tool_history import run
    run(main)
