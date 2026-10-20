#!/usr/bin/env python3
"""Regression checks for the drill preflight and threat mapping."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

RUN = Path(__file__).with_name("run")


class DrillTests(unittest.TestCase):
    def test_requires_threat_model_before_running_suite(self):
        for hq in (None, "", "/nonexistent/cache-security-hq"):
            with self.subTest(hq=hq):
                env = dict(os.environ)
                env.pop("HQ", None)
                if hq is not None:
                    env["HQ"] = hq
                result = subprocess.run(
                    ["bash", str(RUN)], env=env, capture_output=True, text=True,
                    timeout=10,
                )
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("threat model", result.stderr)
                self.assertNotIn("running free/ci", result.stderr)

    def test_mapping(self):
        with tempfile.TemporaryDirectory() as tmp:
            model = Path(tmp) / "model.md"
            matrix = Path(tmp) / "matrix.yaml"
            model.write_text("".join(f"| TM-{n} | threat |\n" for n in range(12, 19)))
            valid = "".join(
                f"  - id: C{n}-LOC-01\n    threat: TM-{n}\n" for n in range(12, 19)
            )
            for name, contents, expected in (
                ("valid", valid, 0),
                ("empty", "", 1),
                ("missing threat", valid.replace("    threat: TM-12\n", ""), 1),
                ("unknown threat", valid.replace("TM-12", "TM-99"), 1),
                ("regex threat", valid + "  - id: extra\n    threat: TM-1[2]\n", 1),
                ("uncovered row", valid.split("  - id: C18")[0], 1),
            ):
                with self.subTest(name=name):
                    matrix.write_text(contents)
                    result = subprocess.run(
                        ["bash", str(RUN), "--check-mapping", str(model), str(matrix)],
                        capture_output=True, text=True, timeout=10,
                    )
                    self.assertEqual(result.returncode, expected, result.stderr)


if __name__ == "__main__":
    unittest.main()
