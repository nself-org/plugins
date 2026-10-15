"""Regression guards for migration-owned schema in the three converted plugins."""
import hashlib
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]
PLUGINS = ("torrent-manager", "vpn", "web3")


class MigrationSchemaTests(unittest.TestCase):
    def test_runtime_does_not_initialize_schema(self):
        for plugin in PLUGINS:
            for directory in ("internal", "cmd"):
                for path in (ROOT / "free" / plugin / directory).glob("*.go"):
                    if path.name.endswith("_test.go"):
                        continue
                    with self.subTest(path=path.relative_to(ROOT)):
                        self.assertIsNone(re.search(r"\bInitSchema\b", path.read_text()),
                                          "runtime schema initialization must be removed")

    def test_vpn_views_are_versioned_and_keep_legacy_semantics(self):
        migration = ROOT / "free/vpn/migrations/002_views.sql"
        self.assertTrue(migration.is_file(), "VPN views need a new migration")
        sql = migration.read_text()
        self.assertNotRegex(sql, r"\b\w+\.public\.")
        views = re.findall(r"CREATE OR REPLACE VIEW\s+(.*?)\s+AS\s+(.*?);", sql, re.S)
        self.assertEqual(len(views), 4)
        expected = {
            # Hashes freeze the complete legacy SELECTs, including column order,
            # account-scoped joins, aggregates and filters, for safe replacement.
            "vpn_active_connections": "4c207d441ec6b54fc5a845fb364dda4ccb39fbd0a067023e660174cd0aa26228",
            "vpn_server_stats": "fd990fc8faa57c45371d44f23444dccf4c5dad9659cb3fd96fe6fa530f46bfd1",
            "vpn_download_history": "8249aeb8ad539af5b46e492687fd279d5026975c87a5a47a4ecfed3be8880368",
            "vpn_provider_uptime": "0b636570b6064774ad4e6a0e46ecd617f8b74a4b84d622746af6931646e59153",
        }
        self.assertEqual({name for name, _ in views}, {"public." + name for name in expected})
        for name, select in views:
            with self.subTest(view=name):
                self.assertTrue(name.startswith("public.vpn_"))
                self.assertNotRegex(select, r"(?<!public\.)\bnp_vpn_\w+")
                legacy = f"CREATE OR REPLACE VIEW {name} AS {select}".replace("public.", "")
                digest = hashlib.sha256(" ".join(legacy.split()).encode()).hexdigest()
                self.assertEqual(digest, expected[name.removeprefix("public.")])


if __name__ == "__main__":
    unittest.main()
