"""Verify packed assets match their sources and stale archives are rejected."""
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import pack_web


class PackedWebTest(unittest.TestCase):
    def test_archive_round_trip_and_source_changes_invalidate_it(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            web, archive = root / "web", root / "assets.zip"
            (web / "vendor").mkdir(parents=True)
            (web / "index.html").write_text("<title>fixture</title>")
            asset = web / "vendor/example.js"
            asset.write_text("export const value = 1;")
            with patch.object(pack_web, "WEB", web), patch.object(pack_web, "ARCHIVE", archive):
                files = pack_web.source_files()
                data = pack_web.archive_bytes(files)
                self.assertEqual(data, pack_web.archive_bytes(files))
                archive.write_bytes(data)
                self.assertTrue(pack_web.matches(files))
                asset.write_text("export const value = 2;")
                self.assertFalse(pack_web.matches(files))
                archive.write_bytes(pack_web.archive_bytes(files))
                with zipfile.ZipFile(archive) as packed:
                    self.assertEqual(packed.read("vendor/example.js"), asset.read_bytes())

    def test_pack_rejects_symlinks_and_ignores_hidden_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "visible.js").write_text("valid")
            (root / ".private").write_text("excluded")
            with patch.object(pack_web, "WEB", root):
                self.assertEqual([path.name for path in pack_web.source_files()], ["visible.js"])
                try:
                    (root / "linked.js").symlink_to(root / "visible.js")
                except OSError:
                    self.skipTest("symbolic links unavailable")
                with self.assertRaises(ValueError):
                    pack_web.source_files()


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
