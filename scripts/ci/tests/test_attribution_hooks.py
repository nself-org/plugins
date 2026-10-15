"""D-0817: tracked hooks scan the staged index in a constant number of calls."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]


class AttributionHooks(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        self.git("init", "-q")
        for directory in (".github/hooks", ".github/scripts"):
            (self.repo / directory).mkdir(parents=True, exist_ok=True)
        for name in ("hooks/pre-commit", "hooks/commit-msg",
                     "scripts/no-attribution-check.sh"):
            shutil.copyfile(ROOT / ".github" / name, self.repo / ".github" / name)

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.repo, check=True,
                              capture_output=True, text=True)

    def hook(self, name, *args, env=None):
        return subprocess.run(["bash", f".github/hooks/{name}", *args],
                              cwd=self.repo, env=env, capture_output=True, text=True)

    def test_many_staged_files_use_one_diff_and_one_content_scan(self):
        for number in range(100):
            (self.repo / f"vendor file {number}.txt").write_text("ordinary content\n")
        self.git("add", ".")
        (self.repo / "bin").mkdir()
        wrapper = self.repo / "bin/git"
        wrapper.write_text('#!/bin/sh\nprintf "%s\\n" "$1" >> "$CALL_LOG"\n'
                           'exec "$REAL_GIT" "$@"\n')
        wrapper.chmod(0o755)
        env = dict(os.environ, REAL_GIT=shutil.which("git"),
                   CALL_LOG=str(self.repo / "calls"))
        env["PATH"] = str(self.repo / "bin") + os.pathsep + env["PATH"]
        result = self.hook("pre-commit", env=env)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        calls = (self.repo / "calls").read_text().splitlines()
        self.assertEqual(calls.count("diff"), 1)
        self.assertEqual(calls.count("grep"), 1)

    def test_staged_content_is_checked_even_when_worktree_is_cleaned(self):
        file = self.repo / "space in filename.txt"
        file.write_text("Co-Authored" + "-By: " + "Code" + "x <example@example.test>\n")
        self.git("add", ".")
        file.write_text("ordinary content\n")
        result = self.hook("pre-commit")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("detected in staged content", result.stderr)

    def test_commit_message_rejects_coauthor_trailer(self):
        message = self.repo / "message"
        message.write_text("Fix metadata\n\nCo-Authored" + "-By: Person <person@example.test>\n")
        result = self.hook("commit-msg", str(message))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("violates attribution policy", result.stderr)


if __name__ == "__main__":
    unittest.main()
