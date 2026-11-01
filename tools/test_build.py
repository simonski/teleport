"""Verify build identities against real isolated Git worktrees."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from build import local_version


class BuildIdentityTest(unittest.TestCase):
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
