"""Guard for D-1359: the Docker Hub username is an org/repo SECRET only.

A workflow that reads it as `vars.DOCKERHUB_USERNAME` gets an empty string,
so the login step fails on the first real publish. Every Docker Hub login
must read the username (and token) from `secrets.`, as ci-toolchain-image.yml
does. Runs under unittest (no third-party packages) and under pytest.
"""
import os
import re
import unittest

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..")
WORKFLOWS = os.path.join(ROOT, ".github", "workflows")
VAR_READ = re.compile(r"\$\{\{[^}]*\bvars\.DOCKERHUB_(?:USERNAME|TOKEN)\b")


class DockerHubCredentials(unittest.TestCase):
    def test_no_workflow_reads_dockerhub_credentials_from_vars(self):
        bad = []
        for name in sorted(os.listdir(WORKFLOWS)):
            if not name.endswith((".yml", ".yaml")):
                continue
            with open(os.path.join(WORKFLOWS, name), encoding="utf-8") as fh:
                for no, line in enumerate(fh, 1):
                    if VAR_READ.search(line):
                        bad.append("%s:%d: %s" % (name, no, line.strip()))
        self.assertEqual(bad, [], "Docker Hub credentials are secrets, not vars")

    def test_plugin_images_login_uses_secret_username(self):
        path = os.path.join(WORKFLOWS, "plugin-images.yml")
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
        self.assertIn("username: ${{ secrets.DOCKERHUB_USERNAME }}", text)


if __name__ == "__main__":
    unittest.main()
