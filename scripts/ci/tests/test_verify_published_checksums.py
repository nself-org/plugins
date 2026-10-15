"""Offline release fixtures for the signed published-checksum writer."""

import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]
PLATFORMS = "darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64".split()


class PublishedChecksumsTests(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)
        scripts = self.root / "scripts"
        scripts.mkdir()
        shutil.copy2(ROOT / "scripts/verify-published-checksums.sh", scripts)
        shutil.copytree(ROOT / "scripts/conformance", scripts / "conformance")
        self.manifest = self.root / "free/demo/plugin.json"
        self.manifest.parent.mkdir(parents=True)
        self.manifest.write_text(json.dumps({"binaryName": "nself-demo"}))
        self.releases = self.root / "releases.json"
        self.original = '{"releases": {}}\n'
        self.releases.write_text(self.original)
        self.assets = self.root / "assets"
        self.assets.mkdir()
        self.archive("demo-1.0.0.tar.gz")
        sha = hashlib.sha256((self.assets / "demo-1.0.0.tar.gz").read_bytes()).hexdigest()
        self.signatures = self.root / "signatures.json"
        self.signatures.write_text(json.dumps({"demo": {
            "sha256": sha, "release_signature": {"key_id": "fixture", "sig": "fixture"}
        }}))
        self.bin = self.root / "bin"
        self.bin.mkdir()
        gh = self.bin / "gh"
        gh.write_text('''#!/usr/bin/env bash
set -euo pipefail
case "$2" in
  view)
    case " $* " in *" --json "*) for f in "$FIXTURE_ASSETS"/*; do basename "$f"; done;; esac
    ;;
  download)
    while [ "$1" != "--dir" ]; do shift; done
    cp "$FIXTURE_ASSETS"/* "$2/"
    ;;
  *) exit 2;;
esac
''')
        gh.chmod(0o755)
        self.exception()

    def archive(self, name):
        with tarfile.open(self.assets / name, "w:gz") as archive:
            data = b"fixture bytes\n"
            entry = tarfile.TarInfo("nself-demo")
            entry.size = len(data)
            archive.addfile(entry, io.BytesIO(data))

    def exception(self, rule="cli-targets", plugin="demo", expires=None):
        text = "exceptions:\n"
        if expires:
            text += (f"  - rule: {rule}\n    plugin: {plugin}\n"
                     f"    reason: fixture\n    owner_ref: TEST\n    expires: {expires}\n")
        (self.root / "scripts/conformance/exceptions.yaml").write_text(text)

    def verify(self):
        env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
                   FIXTURE_ASSETS=str(self.assets), CONF_TODAY="2026-10-14")
        return subprocess.run(
            ["bash", str(self.root / "scripts/verify-published-checksums.sh"),
             "--write", "--signatures", str(self.signatures), "v1.0.0"],
            env=env, text=True, capture_output=True, check=False,
        )

    def assert_missing_platforms(self, count=5):
        result = self.verify()
        output = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1, output)
        self.assertIn(f"{count} expected asset(s) are genuinely absent", output)
        self.assertEqual(self.releases.read_text(), self.original)

    def test_binary_without_any_platform_archives_is_rejected(self):
        self.assert_missing_platforms()

    def test_active_source_only_exception_is_accepted(self):
        self.exception(expires="2026-12-31")
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        row = json.loads(self.releases.read_text())["releases"]["demo"]
        self.assertNotIn("platform_checksums", row)
        self.assertEqual(row["release_signature"]["key_id"], "fixture")

    def test_expired_source_only_exception_is_rejected(self):
        self.exception(expires="2026-10-13")
        self.assert_missing_platforms()

    def test_unrelated_exceptions_are_rejected(self):
        for rule, plugin in [("docker-socket", "demo"), ("cli-targets", "other")]:
            with self.subTest(rule=rule, plugin=plugin):
                self.exception(rule=rule, plugin=plugin, expires="2026-12-31")
                self.assert_missing_platforms()

    def test_exception_does_not_allow_partial_binary_release(self):
        self.exception(expires="2026-12-31")
        self.archive("demo-1.0.0-linux-amd64.tar.gz")
        self.assert_missing_platforms(count=4)

    def test_complete_binary_release_records_all_platforms(self):
        for platform in PLATFORMS:
            self.archive(f"demo-1.0.0-{platform}.tar.gz")
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        row = json.loads(self.releases.read_text())["releases"]["demo"]
        self.assertEqual(set(row["platform_checksums"]), set(PLATFORMS))
        self.assertEqual(row["binaries"], ["nself-demo"])

    def test_source_plugin_needs_no_binary_exception(self):
        self.manifest.write_text("{}")
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
