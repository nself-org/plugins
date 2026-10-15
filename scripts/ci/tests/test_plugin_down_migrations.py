"""D-0399: plugin rollback SQL must warn operators before destructive statements.

Runs in the hygiene workflow's existing unittest discovery, for v1 and v2
plugins alike. These are the public migrations/ directories used by the boot
contract, not private migration trees owned by other runners.
"""
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
WARNING = (
    b"-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.\n"
    b"-- Historical SQL only; it may not reverse the current up migration and may destroy data.\n"
    b"-- See .github/wiki/Plugin-Development.md (Migration rollback policy).\n"
)


class PluginDownMigrationTests(unittest.TestCase):
    def test_down_migrations_warn_before_any_sql(self):
        paths = sorted((ROOT / "free").glob("*/migrations/*.down.sql"))
        self.assertTrue(paths, "no plugin down migrations checked")
        for path in paths:
            with self.subTest(path=str(path.relative_to(ROOT))):
                self.assertTrue(
                    path.read_bytes().startswith(WARNING),
                    "unmaintained down SQL must start with the rollback warning",
                )


if __name__ == "__main__":
    unittest.main()
