"""Exercise the Go CI loop with fixture modules and a recording Go command."""

import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


WORKFLOW = Path(__file__).resolve().parents[3] / ".github/workflows/build-test.yml"
MODULES = ("free/plain", "free/nested/go", "sdk", "scripts/conformance/tool")


class GoBuildWorkflow(unittest.TestCase):
    def run_workflow(self, fail_module="", fail_command=""):
        """Run the actual workflow shell; stub compilation, retain discovery."""
        script = textwrap.dedent(WORKFLOW.read_text().split("run: |\n", 1)[1])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            for module in (*MODULES, "sdk/vendor/dep", "free/plain/vendor/dep",
                           "tests/conformance/fixtures/example"):
                path = root / module / "go.mod"
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("module example.test/fixture\n")
            bin_dir = root / "bin"
            bin_dir.mkdir()
            go = bin_dir / "go"
            go.write_text(
                '#!/bin/sh\n'
                'module="${PWD#"$FIXTURE_ROOT"/}"\n'
                'printf "%s|%s\\n" "$module" "$*" >> "$GO_CALLS"\n'
                'if [ "$module" = "$FAIL_MODULE" ] && '
                '[ "$1" = "$FAIL_COMMAND" ]; then exit 1; fi\n'
            )
            go.chmod(0o755)
            log = root / "calls"
            env = dict(os.environ, PATH=str(bin_dir) + os.pathsep + os.environ["PATH"],
                       FIXTURE_ROOT=str(root), GO_CALLS=str(log),
                       FAIL_MODULE=fail_module, FAIL_COMMAND=fail_command)
            result = subprocess.run(["bash", "-e", "-o", "pipefail", "-c", script],
                                    cwd=root, env=env, capture_output=True, text=True)
            return result, log.read_text().splitlines()

    def test_builds_vets_and_tests_plugins_sdk_and_tools(self):
        result, calls = self.run_workflow()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        expected = [f"{module}|{command}" for module in MODULES for command in
                    ("build -o /dev/null ./...", "vet ./...", "test ./...")]
        self.assertCountEqual(calls, expected)

    def test_sdk_and_tool_failures_fail_the_job(self):
        for module in ("sdk", "scripts/conformance/tool"):
            for command in ("build", "vet", "test"):
                with self.subTest(module=module, command=command):
                    result, _ = self.run_workflow(module, command)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertIn(f"FAIL: {module}", result.stdout)


if __name__ == "__main__":
    unittest.main()
