"""D-0670: HTTP-only retro-gaming docs must match the manifest surface."""
import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
DOCS = (".github/wiki/plugins/Retro-Gaming.md", "free/retro-gaming/README.md")


class RetroGamingDocsTests(unittest.TestCase):
    def test_no_unimplemented_cli_commands(self):
        for name in DOCS:
            with self.subTest(document=name):
                text = (ROOT / name).read_text()
                self.assertNotRegex(text, r"nself plugin retro-gaming\b")
                self.assertNotIn("## CLI Commands", text)
                self.assertNotIn("`cores seed`", text)

    def test_rest_tables_match_manifest(self):
        manifest = json.loads((ROOT / "free/retro-gaming/plugin.json").read_text())
        expected = {tuple(e.split(" ", 1)) for e in manifest["apiEndpoints"]}
        health = {("GET", "/health"), ("GET", "/ready")}
        for name in DOCS:
            with self.subTest(document=name):
                text = (ROOT / name).read_text()
                rows = set(re.findall(
                    r"^\|\s*`?(GET|POST|PUT|PATCH|DELETE)`?\s*\|\s*`([^`]+)`",
                    text, re.MULTILINE,
                ))
                self.assertEqual(rows - health, expected)


if __name__ == "__main__":
    unittest.main()
