"""Content fingerprints for optional local build and dependency inventory reuse."""
import hashlib
import json
import os
from pathlib import Path
import subprocess


def digest_file(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def source_identity(root, purpose):
    environment = subprocess.check_output(["go", "env", "-json"], cwd=root)
    module = json.loads(subprocess.check_output(["go", "mod", "edit", "-json"], cwd=root))
    go_environment = json.loads(environment)
    # Go injects a random temporary build directory into this derived value.
    # Stable target/compiler settings and explicit CGO flags are hashed below.
    go_environment.pop("GOGCCFLAGS", None)
    digest = hashlib.sha256(purpose.encode() + json.dumps(go_environment, sort_keys=True).encode())
    # Compiler command overrides and CGO flags may change output independently
    # of tracked sources. Only the digest is persisted.
    digest.update(json.dumps({name: os.environ.get(name, "") for name in (
        "TELEPORT_BUILD_VERSION", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS",
        "CGO_FFLAGS", "CGO_LDFLAGS", "CC", "CXX", "PKG_CONFIG",
    )}, sort_keys=True).encode())
    def include(path, prefix):
        if path.is_file():
            digest.update(prefix.encode() + b"\0" + path.read_bytes() + b"\0")
    for name in ("go.mod", "go.sum", "go.work", "go.work.sum"):
        include(root / name, name)
    for name in ("cmd", "internal"):
        for path in sorted((root / name).rglob("*")):
            include(path, path.relative_to(root).as_posix())
    # Replacements outside this checkout must participate too. A workspace can
    # contain arbitrary other modules, so conservatively disable reuse there.
    if go_environment.get("GOWORK") not in ("", "off", None):
        return None
    if any(flag in go_environment.get("GOFLAGS", "") for flag in ("-overlay", "-modfile", "-toolexec", "-pkgdir")):
        return None
    for replacement in module.get("Replace") or []:
        target = replacement["New"]
        if not target.get("Version"):
            directory = (root / target["Path"]).resolve()
            for path in sorted(directory.rglob("*")):
                if ".git" not in path.parts:
                    include(path, "replacement/" + str(directory) + "/" + path.relative_to(directory).as_posix())
    return digest.hexdigest()


def verified_artifact(manifest, identity, artifact):
    if identity is None:
        return False
    try:
        record = json.loads(manifest.read_text())
        return record["identity"] == identity and record["sha256"] == digest_file(artifact)
    except (OSError, ValueError, KeyError):
        return False


def record_artifact(manifest, identity, artifact):
    manifest.parent.mkdir(parents=True, exist_ok=True)
    staged = manifest.with_suffix(".tmp")
    staged.write_text(json.dumps({"identity": identity, "sha256": digest_file(artifact)}))
    staged.replace(manifest)
