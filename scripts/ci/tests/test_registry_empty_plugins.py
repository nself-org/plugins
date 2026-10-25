"""Gate: the registry never serves a plugin that has no implementation (D-1458).

A registry entry with a tarball URL is installable bytes, signed as if shipped. A plugin whose free/<slug>
directory holds only its manifest and docs (plugin.json, README.md, SPEC.md ...) has no handlers, migrations,
compose fragment or source: `nself add <slug>` would install a plugin that does nothing. Rules:

  * a registry entry that carries a tarball for an empty plugin fails, unless it is listed in EMPTY_SERVED;
  * a listed slug that gained an implementation, or left the registry, fails until its line is deleted, so
    the list only shrinks (same ratchet shape as test_plugin_nginx_server_ratchet.py).

EMPTY_SERVED is debt, not approval. The fix for a slug is to drop its row from releases.json and regenerate
(scripts/plugin-counts.sh), as was done for transactional-email.

Run: python3 -m unittest discover -s scripts/ci/tests -p 'test_registry_empty_plugins.py'
No docker, network or third-party packages.
"""
import json
import os
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))

# Files that describe a plugin without implementing it.
DOC_FILES = {"plugin.json", "README.md", "SPEC.md", "LICENSE", "CHANGELOG.md"}

# Still served as installable bytes with nothing behind them. Remove a line when the plugin is implemented
# or its release row is dropped.
EMPTY_SERVED = {
    "family-familysearch",
    "family-myheritage",
    "family-wikitree",
    "nself-geo",
    "nself-image",
    "nself-pdf",
    "nself-scan",
}


def is_empty(slug):
    """True when free/<slug> is missing or holds nothing but manifest and docs."""
    base = os.path.join(ROOT, "free", slug)
    if not os.path.isdir(base):
        return True
    for _, _, files in os.walk(base):
        if any(f not in DOC_FILES for f in files):
            return False
    return True


def served_empty(registry):
    """Slugs whose registry entry carries a tarball while the plugin directory is empty."""
    plugins = registry["plugins"]
    return sorted(
        slug for slug, entry in plugins.items()
        if (entry.get("tarball") or entry.get("download_url")) and is_empty(slug)
    )


class RegistryEmptyPlugins(unittest.TestCase):
    def setUp(self):
        with open(os.path.join(ROOT, "registry.json")) as f:
            self.registry = json.load(f)

    def test_no_new_empty_plugin_is_served(self):
        new = [s for s in served_empty(self.registry) if s not in EMPTY_SERVED]
        self.assertEqual(
            new, [],
            "registry.json serves a tarball for a plugin with no implementation; drop its row from "
            "releases.json and run scripts/plugin-counts.sh: %s" % ", ".join(new))

    def test_empty_served_list_only_shrinks(self):
        still = set(served_empty(self.registry))
        stale = sorted(EMPTY_SERVED - still)
        self.assertEqual(
            stale, [],
            "delete these from EMPTY_SERVED, they are implemented or no longer served: %s" % ", ".join(stale))

    def test_detector_flags_a_docs_only_directory(self):
        # The gate must be able to fail: a docs-only plugin dir is empty, one with a source file is not.
        import tempfile
        global ROOT
        saved = ROOT
        try:
            with tempfile.TemporaryDirectory() as tmp:
                ROOT = tmp
                os.makedirs(os.path.join(tmp, "free", "docs-only"))
                os.makedirs(os.path.join(tmp, "free", "real", "migrations"))
                for name in ("plugin.json", "README.md", "SPEC.md"):
                    open(os.path.join(tmp, "free", "docs-only", name), "w").close()
                open(os.path.join(tmp, "free", "real", "plugin.json"), "w").close()
                open(os.path.join(tmp, "free", "real", "migrations", "001_init.up.sql"), "w").close()
                reg = {"plugins": {
                    "docs-only": {"tarball": "https://example.invalid/a.tar.gz"},
                    "real": {"tarball": "https://example.invalid/b.tar.gz"},
                    "hidden": {},
                }}
                self.assertEqual(served_empty(reg), ["docs-only"])
        finally:
            ROOT = saved


if __name__ == "__main__":
    unittest.main()
