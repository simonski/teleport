"""Create or verify the reproducible compressed web assets embedded in tk."""
import argparse
import io
import os
from pathlib import Path
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "internal/server/web"
ARCHIVE = ROOT / "internal/server/web_assets.zip"


def source_files():
    result = []
    for path in sorted(WEB.rglob("*")):
        if any(part.startswith(".") for part in path.relative_to(WEB).parts):
            continue
        if path.is_symlink():
            raise ValueError("Web assets must not contain symlinks")
        if path.is_file():
            result.append(path)
    return result


def archive_bytes(files):
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for path in files:
            info = zipfile.ZipInfo(path.relative_to(WEB).as_posix(), (1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, path.read_bytes(), compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
    return output.getvalue()


def matches(files):
    try:
        with zipfile.ZipFile(ARCHIVE) as archive:
            names = [path.relative_to(WEB).as_posix() for path in files]
            return archive.namelist() == names and all(archive.read(name) == path.read_bytes() for name, path in zip(names, files))
    except (OSError, zipfile.BadZipFile, RuntimeError):
        return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    files = source_files()
    valid = matches(files)
    if args.verify:
        if not valid:
            raise SystemExit("Web asset archive is stale; run python3 -B tools/pack_web.py")
        print("PASS: compressed web assets match every source file")
    else:
        if not valid:
            content = archive_bytes(files)
            descriptor, temporary = tempfile.mkstemp(prefix=".web-assets-", dir=ARCHIVE.parent)
            try:
                with os.fdopen(descriptor, "wb") as output:
                    output.write(content)
                os.replace(temporary, ARCHIVE)
            finally:
                Path(temporary).unlink(missing_ok=True)
        print(f"Packed web assets: {ARCHIVE.stat().st_size} bytes")


if __name__ == "__main__":
    from tool_history import run
    run(main)
