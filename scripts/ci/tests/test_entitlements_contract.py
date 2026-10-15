"""D-0817: published metadata must describe the boot schema and service port."""
import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
PLUGIN = ROOT / "free/entitlements"


class EntitlementsContract(unittest.TestCase):
    def setUp(self):
        self.manifest = json.loads((PLUGIN / "plugin.json").read_text())

    def test_tables_match_boot_migrations(self):
        tables = []
        for migration in sorted((PLUGIN / "migrations").glob("*.up.sql")):
            tables.extend(re.findall(
                r"CREATE TABLE IF NOT EXISTS ([\w.]+)", migration.read_text()))
        declared = [f"{self.manifest['schema']}.{table}"
                    for table in self.manifest["tables"]]
        self.assertEqual(sorted(declared), sorted(tables))

    def test_container_ports_match_manifest(self):
        port = self.manifest["port"]
        self.assertEqual(self.manifest["service"]["port"], port)
        dockerfile = (PLUGIN / "Dockerfile").read_text()
        self.assertRegex(dockerfile, rf"(?m)^EXPOSE {port}$")
        compose = (PLUGIN / "docker-compose.plugin.yml").read_text()
        self.assertIn(f'"127.0.0.1:{port}:{port}"', compose)
        self.assertIn(f'PORT: "{port}"', compose)
        self.assertIn(f"http://127.0.0.1:{port}/health", compose)
        self.assertNotIn("3715", (PLUGIN / "nginx/entitlements.conf").read_text())


if __name__ == "__main__":
    unittest.main()
