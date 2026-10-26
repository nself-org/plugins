"""Tests for the triggers of .github/workflows/registry-sync.yml (D-1480).

The worker's served catalog only moves when POST /api/sync runs. The daily schedule stays, and a push to main that
changes a projection the drill compares (catalog.json, registry.json, counts.json) must run it too, or
drill-catalog-freshness stays red from the merge until the next daily run. The workflow is read as text (no YAML
library on the runner), so this checks the `on:` block only.
"""

from pathlib import Path
import re
import unittest

WORKFLOW = Path(__file__).resolve().parents[3] / ".github" / "workflows" / "registry-sync.yml"
SERVED = {"catalog.json", "registry.json", "counts.json"}


def on_block(text):
    m = re.search(r"^on:\n((?:[ \t]+.*\n|[ \t]*\n)+)", text, re.M)
    assert m, "no top-level on: block"
    return m.group(1)


def section(block, key):
    """The lines indented under `  key:` inside the on: block."""
    m = re.search(r"^  " + key + r":\n((?:    .*\n|[ \t]*\n)+)", block, re.M)
    return m.group(1) if m else None


class RegistrySyncTriggers(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.block = on_block(self.text)

    def test_daily_schedule_stays(self):
        sched = section(self.block, "schedule")
        self.assertIsNotNone(sched, "the daily schedule is gone")
        self.assertRegex(sched, r"cron:\s*'17 5 \* \* \*'")

    def test_push_to_main_on_served_projections(self):
        push = section(self.block, "push")
        self.assertIsNotNone(push, "no push trigger")
        self.assertRegex(push, r"branches:\s*\[main\]")
        paths = set(re.findall(r"^\s+-\s+['\"]?([^'\"\s]+)['\"]?\s*$", push.split("paths:")[1], re.M))
        self.assertEqual(paths, SERVED)

    def test_no_new_secret(self):
        secrets = set(re.findall(r"secrets\.([A-Z0-9_]+)", self.text))
        self.assertEqual(secrets, {"CLOUDFLARE_WORKER_SYNC_TOKEN"})


if __name__ == "__main__":
    unittest.main()
