"""Verify build identities against real isolated Git worktrees."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from build import install_artifact, local_version


class BuildIdentityTest(unittest.TestCase):
    @unittest.skipUnless(os.name == "posix", "POSIX permits a running executable's directory entry to be replaced")
    def test_install_replaces_the_path_without_changing_open_binary_bytes(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            source, destination = folder / "new", folder / "tk"
            source.write_bytes(b"new executable")
            source.chmod(0o755)
            destination.write_bytes(b"running executable")
            with destination.open("rb") as running:
                install_artifact(source, destination)
                self.assertEqual(running.read(), b"running executable")
            self.assertEqual(destination.read_bytes(), b"new executable")
            self.assertEqual(destination.stat().st_mode & 0o777, 0o755)
            self.assertFalse(list(folder.glob(".tk-install-*")))

    def test_failed_install_retains_existing_binary_and_cleans_temporary_copy(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            source, destination = folder / "new", folder / "tk"
            source.write_bytes(b"new executable")
            destination.write_bytes(b"existing executable")
            with patch("build.shutil.copy2", side_effect=OSError("fixture copy failure")):
                with self.assertRaises(OSError):
                    install_artifact(source, destination)
            self.assertEqual(destination.read_bytes(), b"existing executable")
            self.assertFalse(list(folder.glob(".tk-install-*")))

    def test_clean_modified_untracked_and_ignored_sources(self):
        with tempfile.TemporaryDirectory(prefix="teleport-build-test-") as temporary:
            folder = Path(temporary)
            environment = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
            environment.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_SYSTEM=os.devnull)
            def git(*arguments):
                subprocess.run(["git", "-c", f"core.hooksPath={os.devnull}", "-c", "commit.gpgSign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", *arguments], cwd=folder, env=environment, check=True, capture_output=True)
            git("init", "-b", "main")
            source = folder / "source.txt"
            source.write_text("original\n")
            (folder / ".gitignore").write_text("bin/\n.build-count\n")
            git("add", ".")
            git("commit", "-m", "Initial fixture")
            self.assertRegex(local_version(folder, 7), r"^0\.1\.0-dev\.7\+[0-9a-f]+$")
            source.write_text("changed\n")
            self.assertTrue(local_version(folder, 8).endswith(".dirty"))
            git("checkout", "--", "source.txt")
            untracked = folder / "new-source.txt"
            untracked.write_text("new\n")
            self.assertTrue(local_version(folder, 9).endswith(".dirty"))
            untracked.unlink()
            (folder / "bin").mkdir()
            (folder / "bin/tk").write_text("generated binary fixture")
            (folder / ".build-count").write_text("9")
            self.assertFalse(local_version(folder, 10).endswith(".dirty"))


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
