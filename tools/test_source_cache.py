"""Verify artifact integrity and invalidation of source, embedded assets and flags."""
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from source_cache import source_identity, verified_artifact, record_artifact


class SourceCacheTest(unittest.TestCase):
    def test_input_changes_invalidate_and_tampering_is_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "internal/web").mkdir(parents=True)
            asset = root / "internal/web/app.js"
            asset.write_text("original")
            (root / "go.mod").write_text("module fixture")
            def command(args, **kwargs):
                return json.dumps({"GOOS": "darwin", "GOWORK": "off"} if args[1] == "env" else {}).encode()
            with patch("source_cache.subprocess.check_output", side_effect=command):
                identity = source_identity(root, "fixture")
                self.assertEqual(identity, source_identity(root, "fixture"))
                asset.write_text("changed embedded bytes")
                self.assertNotEqual(identity, source_identity(root, "fixture"))
                asset.write_text("original")
                with patch.dict(os.environ, CGO_CFLAGS="-DOTHER"):
                    self.assertNotEqual(identity, source_identity(root, "fixture"))
                self.assertNotEqual(identity, source_identity(root, "new generator"))
            artifact, manifest = root / "binary", root / "manifest.json"
            artifact.write_text("verified output")
            record_artifact(manifest, identity, artifact)
            self.assertTrue(verified_artifact(manifest, identity, artifact))
            artifact.write_text("tampered")
            self.assertFalse(verified_artifact(manifest, identity, artifact))
            self.assertFalse(verified_artifact(manifest, None, artifact))

    def test_external_local_replacement_changes_invalidate(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "replacement").mkdir()
            source = root / "replacement/dep.go"
            source.write_text("before")
            module = {"Replace": [{"New": {"Path": "./replacement"}}]}
            with patch("source_cache.subprocess.check_output", side_effect=[b'{}', json.dumps(module).encode()] * 2):
                first = source_identity(root, "fixture")
                source.write_text("after")
                self.assertNotEqual(first, source_identity(root, "fixture"))


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
