"""Keep uninstall's platform-specific confirmation requirements documented."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]


class K8sUninstallDocsTests(unittest.TestCase):
    def test_uninstall_confirmation_platforms(self):
        wiki = (ROOT / ".github/wiki/plugins/K8s.md").read_text()
        section = wiki.split("## Uninstall\n", 1)[1].split("\n## ", 1)[0]
        section = re.sub(r"\s+", " ", section)
        self.assertRegex(
            section,
            r"Without `--yes`, a terminal on Linux or macOS .*release name",
        )
        self.assertRegex(
            section,
            r"On Windows and other operating systems, .*requires `--yes` "
            r"\(or `-y`\).*no .*prompt",
        )


if __name__ == "__main__":
    unittest.main()
