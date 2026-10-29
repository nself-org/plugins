"""Tests for scripts/ci/go-test-ran.py.

The script reads `go test -json` events on stdin and exits 0 only when every
named test prefix has a passing test. Unrelated skips must not fail it (a
package run such as ./cmd/... always carries some); a failure anywhere, or a
named test that was skipped, must.
Runs under unittest (no third-party packages) and under pytest.
"""
import json
import os
import subprocess
import sys
import unittest

SCRIPT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..",
                      "go-test-ran.py")


def ev(action, test=None, package="example.com/p"):
    event = {"Action": action, "Package": package}
    if test:
        event["Test"] = test
    return json.dumps(event)


def run(lines, *prefixes):
    proc = subprocess.run([sys.executable, SCRIPT, *prefixes],
                          input="\n".join(lines).encode(),
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    return proc.returncode


class GoTestRan(unittest.TestCase):
    def test_named_tests_pass(self):
        lines = [ev("run", "TestA"), ev("pass", "TestA"), ev("pass")]
        self.assertEqual(run(lines, "TestA"), 0)

    def test_unrelated_skips_do_not_fail(self):
        lines = [ev("pass", "TestA"), ev("skip", "TestOther"),
                 ev("skip", "TestOther2/sub"), ev("skip", package="example.com/empty"),
                 ev("pass")]
        self.assertEqual(run(lines, "TestA"), 0)

    def test_named_test_skipped_fails(self):
        lines = [ev("skip", "TestA"), ev("pass", "TestB")]
        self.assertEqual(run(lines, "TestA", "TestB"), 1)

    def test_named_subtest_skipped_fails(self):
        lines = [ev("pass", "TestA/one"), ev("skip", "TestA/two"), ev("pass", "TestA")]
        self.assertEqual(run(lines, "TestA"), 1)

    def test_named_test_never_ran_fails(self):
        lines = [ev("pass", "TestB"), ev("pass")]
        self.assertEqual(run(lines, "TestA"), 1)

    def test_every_prefix_needs_a_pass(self):
        lines = [ev("pass", "TestA")]
        self.assertEqual(run(lines, "TestA", "TestB"), 1)

    def test_prefix_matches_a_passing_subtest(self):
        self.assertEqual(run([ev("pass", "TestLinuxLeak/found")], "TestLinux"), 0)

    def test_failed_named_test_fails(self):
        lines = [ev("pass", "TestA/one"), ev("fail", "TestA")]
        self.assertEqual(run(lines, "TestA"), 1)

    def test_failed_unrelated_test_fails(self):
        lines = [ev("pass", "TestA"), ev("fail", "TestOther")]
        self.assertEqual(run(lines, "TestA"), 1)

    def test_failed_package_fails(self):
        lines = [ev("pass", "TestA"), ev("fail")]
        self.assertEqual(run(lines, "TestA"), 1)

    def test_non_json_line_fails(self):
        lines = [ev("pass", "TestA"), "# example.com/p [build failed]"]
        self.assertEqual(run(lines, "TestA"), 1)

    def test_empty_input_fails(self):
        self.assertEqual(run([], "TestA"), 1)

    def test_no_prefix_is_a_usage_error(self):
        self.assertNotEqual(run([ev("pass", "TestA")]), 0)


if __name__ == "__main__":
    unittest.main()
