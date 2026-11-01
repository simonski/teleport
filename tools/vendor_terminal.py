"""Bundle pinned xterm.js assets with verified npm archives and local hashes."""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile

from vendor_web import download

DESTINATION = Path(__file__).resolve().parents[1] / "internal/server/web/vendor/terminal"
PACKAGES = (
    ("@xterm/xterm", "6.0.0", {"package/lib/xterm.js": "xterm.js", "package/css/xterm.css": "xterm.css", "package/LICENSE": "xterm-LICENSE"}),
    ("@xterm/addon-fit", "0.11.0", {"package/lib/addon-fit.js": "addon-fit.js", "package/LICENSE": "addon-fit-LICENSE"}),
)


def verify():
    lock = json.loads((DESTINATION / "manifest.json").read_text())
    assert [(p["name"], p["version"]) for p in lock["packages"]] == [(p[0], p[1]) for p in PACKAGES]
    for name, expected in lock["sha256"].items():
        assert hashlib.sha256((DESTINATION / name).read_bytes()).hexdigest() == expected, name
    print("PASS: bundled " + ", ".join(f"{name} {version}" for name, version, _ in PACKAGES) + " hashes")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    if args.verify:
        verify()
        return
    DESTINATION.mkdir(parents=True, exist_ok=True)
    lock = {"packages": [], "sha256": {}}
    for name, version, files in PACKAGES:
        metadata = json.loads(download(f"https://registry.npmjs.org/{name}/{version}"))
        distribution = metadata["dist"]
        archive = download(distribution["tarball"])
        algorithm, expected = distribution["integrity"].split("-", 1)
        assert algorithm == "sha512"
        assert base64.b64encode(hashlib.sha512(archive).digest()).decode() == expected
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as package:
            for source, filename in files.items():
                content = package.extractfile(source).read()
                (DESTINATION / filename).write_bytes(content)
                lock["sha256"][filename] = hashlib.sha256(content).hexdigest()
        lock["packages"].append({"name": name, "version": version, "integrity": distribution["integrity"], "source": distribution["tarball"]})
    (DESTINATION / "manifest.json").write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n")
    verify()


if __name__ == "__main__":
    main()
