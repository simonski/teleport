"""Fetch pinned Three.js browser modules, verify npm integrity, and record local hashes.

Bundled modules are served by tk without a CDN or Node runtime dependency.
Use --verify for an offline artifact check.
"""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
DESTINATION = ROOT / "internal/server/web/vendor"
VERSION = "0.187.0"
FILES = {
    "package/build/three.module.js": "three.module.js",
    "package/build/three.core.js": "three.core.js",
    "package/examples/jsm/controls/OrbitControls.js": "OrbitControls.js",
    "package/LICENSE": "LICENSE",
}


def download(url):
    with urllib.request.urlopen(url, timeout=30) as response:
        return response.read()


def verify():
    lock = json.loads((DESTINATION / "manifest.json").read_text())
    assert lock["version"] == VERSION
    assert set(lock["sha256"]) == set(FILES.values())
    for name, expected in lock["sha256"].items():
        assert hashlib.sha256((DESTINATION / name).read_bytes()).hexdigest() == expected, name
    print(f"PASS: bundled Three.js {VERSION} file hashes")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    if args.verify:
        verify()
        return
    metadata = json.loads(download(f"https://registry.npmjs.org/three/{VERSION}"))
    distribution = metadata["dist"]
    archive = download(distribution["tarball"])
    algorithm, expected = distribution["integrity"].split("-", 1)
    assert algorithm == "sha512"
    assert base64.b64encode(hashlib.sha512(archive).digest()).decode() == expected
    DESTINATION.mkdir(exist_ok=True)
    hashes = {}
    contents = {}
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as package:
        for source, name in FILES.items():
            content = package.extractfile(source).read()
            if name == "OrbitControls.js":
                assert b"from 'three';" in content
                content = content.replace(b"from 'three';", b"from './three.module.js';")
            contents[name] = content
            hashes[name] = hashlib.sha256(content).hexdigest()
    for name, content in contents.items():
        (DESTINATION / name).write_bytes(content)
    # Upstream no longer publishes the minified builds; remove obsolete copies.
    for name in ("three.module.min.js", "three.core.min.js"):
        (DESTINATION / name).unlink(missing_ok=True)
    lock = {"package": "three", "version": VERSION, "integrity": distribution["integrity"], "source": distribution["tarball"], "sha256": hashes, "patch": "OrbitControls imports the bundled relative module"}
    (DESTINATION / "manifest.json").write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n")
    verify()


if __name__ == "__main__":
    main()
