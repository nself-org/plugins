"""D-0821: the advertised workflow ports must reach the container listener."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


class WorkflowsPortsTest(unittest.TestCase):
    def test_manifest_matches_listener(self):
        plugin = ROOT / "free/workflows"
        manifest = json.loads((plugin / "plugin.json").read_text())
        docker = (plugin / "Dockerfile").read_text()
        compose = (plugin / "docker-compose.plugin.yml").read_text()
        config = (plugin / "internal/config.go").read_text()
        listener = int(re.search(r"(?m)^EXPOSE (\d+)$", docker)[1])
        for name, port in (
            ("port", manifest["port"]),
            ("service.port", manifest["service"]["port"]),
            ("compose PORT", int(re.search(r'PORT: "(\d+)"', compose)[1])),
            ("config default", int(re.search(r'port = "(\d+)"', config)[1])),
        ):
            with self.subTest(field=name):
                self.assertEqual(port, listener)
        self.assertIn(f'"127.0.0.1:{listener}:{listener}"', compose)
        self.assertIn(f"http://127.0.0.1:{listener}/health", compose)


if __name__ == "__main__":
    unittest.main()
