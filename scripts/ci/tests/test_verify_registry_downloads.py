"""Offline fixtures for public download verification (D-1483)."""

import hashlib
import importlib.util
import io
from pathlib import Path
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "verify_registry_downloads.py"
SPEC = importlib.util.spec_from_file_location("verify_registry_downloads", SCRIPT)
VERIFIER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VERIFIER)
PUBLISHED = b"draft release bytes, never a local rebuild"
SHA = hashlib.sha256(PUBLISHED).hexdigest()


class Response(io.BytesIO):
    def geturl(self):
        return "https://cdn.example/plugin.tar.gz?token=temporary#secret"


class RegistryDownloads(unittest.TestCase):
    def row(self, name="demo", **overrides):
        return dict(name=name, tier="free", checksum=SHA,
                    download_url=f"https://registry.example/{name}/tarball", **overrides)

    def run_verify(self, plugins, data=PUBLISHED):
        self.urls = []
        self.output = []

        def opener(url):
            self.urls.append(url)
            return Response(data)

        return VERIFIER.verify({"plugins": plugins}, opener, self.output.append)

    def test_keyed_and_array_formats(self):
        for plugins in ([self.row()], {"demo": self.row()}):
            self.assertEqual(self.run_verify(plugins), 0)
            self.assertEqual(self.urls, [self.row()["download_url"]])

    def test_worker_url_wins_over_release_fallback(self):
        row = self.row(tarball="https://github.example/draft.tar.gz")
        self.assertEqual(self.run_verify([row]), 0)
        self.assertEqual(self.urls, [row["download_url"]])

    def test_tarball_fallback(self):
        row = self.row()
        del row["download_url"]
        row["tarball"] = "https://github.example/draft.tar.gz"
        self.assertEqual(self.run_verify([row]), 0)
        self.assertEqual(self.urls, [row["tarball"]])

    def test_uppercase_tarball_url_alias(self):
        row = self.row()
        del row["download_url"]
        row["tarballURL"] = "https://github.example/draft.tar.gz"
        self.assertEqual(self.run_verify([row]), 0)
        self.assertEqual(self.urls, [row["tarballURL"]])

    def test_final_url_has_no_temporary_query_or_fragment(self):
        for data in (PUBLISHED, b"wrong bytes"):
            self.run_verify([self.row()], data)
            self.assertIn("https://cdn.example/plugin.tar.gz", self.output[0])
            self.assertNotIn("token=", self.output[0])
            self.assertNotIn("#secret", self.output[0])

    def test_rebuilt_or_corrupted_bytes_fail(self):
        self.assertEqual(self.run_verify([self.row()], b"rebuilt bytes"), 1)
        self.assertIn("SHA256 mismatch", self.output[0])

    def test_all_checksum_aliases_are_checked(self):
        row = self.row(checksums={"sha256": "0" * 64})
        self.assertEqual(self.run_verify([row]), 1)

    def test_missing_invalid_checksum_and_url_fail(self):
        for field, value in (("checksum", None), ("checksum", "bad"),
                             ("download_url", None), ("download_url", "http://example/x")):
            row = self.row()
            row[field] = value
            self.assertEqual(self.run_verify([row]), 1)
            self.assertEqual(self.urls, [])

    def test_paid_and_noninstallable_entries_never_downloaded(self):
        rows = [self.row(), self.row("disabled", installable=False),
                self.row("licensed", requires_license=True)]
        paid = self.row("paid")
        paid["tier"] = "max"
        self.assertEqual(self.run_verify(rows + [paid]), 0)
        self.assertEqual(len(self.urls), 1)

    def test_failure_does_not_skip_other_plugins(self):
        bad = self.row("bad")
        bad["checksum"] = "bad"
        self.assertEqual(self.run_verify([bad, self.row()]), 1)
        self.assertTrue(any(line.startswith("OK demo") for line in self.output))

    def test_empty_registry_fails(self):
        self.assertEqual(self.run_verify([]), 1)

    def test_download_failure_fails(self):
        def unavailable(url):
            raise OSError("HTTP 503")
        output = []
        self.assertEqual(VERIFIER.verify({"plugins": [self.row()]}, unavailable, output.append), 1)
        self.assertIn("HTTP 503", output[0])


if __name__ == "__main__":
    unittest.main()
