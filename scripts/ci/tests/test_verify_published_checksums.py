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
        shutil.copy2(ROOT / "scripts/verify-published-checksums-lib.sh", scripts)
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

    def archive(self, name, marker=""):
        with tarfile.open(self.assets / name, "w:gz") as archive:
            data = b"fixture bytes\n" + marker.encode()
            entry = tarfile.TarInfo("nself-demo")
            entry.size = len(data)
            archive.addfile(entry, io.BytesIO(data))

    def exception(self, rule="cli-targets", plugin="demo", expires=None):
        text = "exceptions:\n"
        if expires:
            text += (f"  - rule: {rule}\n    plugin: {plugin}\n"
                     f"    reason: fixture\n    owner_ref: TEST\n    expires: {expires}\n")
        (self.root / "scripts/conformance/exceptions.yaml").write_text(text)

    def sha(self, name):
        return hashlib.sha256((self.assets / name).read_bytes()).hexdigest()

    def sign_platforms(self, platforms=PLATFORMS, sha=None):
        """Add a platform signature per platform to the signatures file; sha overrides the covered hash."""
        doc = json.loads(self.signatures.read_text())
        doc["demo"]["platform_signatures"] = {
            p: {"sha256": sha or self.sha(f"demo-1.0.0-{p}.tar.gz"),
                "key_id": "fixture", "alg": "ed25519", "sig": f"fixture-{p}"}
            for p in platforms
        }
        self.signatures.write_text(json.dumps(doc))

    def approve(self, overrides=None):
        """Write the publish job's approved manifest (sha256sum lines of every served tarball); overrides maps name -> sha."""
        lines = {f.name: self.sha(f.name) for f in sorted(self.assets.glob("*.tar.gz"))}
        lines.update(overrides or {})
        manifest = self.root / "published-sha256.txt"
        manifest.write_text("".join(f"{sha}  {name}\n" for name, sha in lines.items()))
        return manifest

    def verify(self, signed=True, published="approve"):
        """Run the writer. A signed run also needs the approved manifest: "approve" (default) approves exactly what is served,
        None omits --published-sha256, a Path uses that manifest."""
        env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
                   FIXTURE_ASSETS=str(self.assets), CONF_TODAY="2026-10-14")
        args = ["--write"]
        if signed:
            args += ["--signatures", str(self.signatures)]
            if published == "approve":
                published = self.approve()
            if published is not None:
                args += ["--published-sha256", str(published)]
        return subprocess.run(
            ["bash", str(self.root / "scripts/verify-published-checksums.sh"), *args, "v1.0.0"],
            env=env, text=True, capture_output=True, check=False,
        )

    def archive_all(self):
        for platform in PLATFORMS:
            self.archive(f"demo-1.0.0-{platform}.tar.gz", marker=platform)

    def seed_row(self, signatures=True, **extra):
        """A releases.json row already on v1.0.0, as a previous --write left it."""
        row = {"version": "1.0.0", "release_tag": "v1.0.0", "sha256": self.sha("demo-1.0.0.tar.gz"),
               "release_signature": {"key_id": "fixture", "alg": "ed25519", "sig": "src"},
               "platform_checksums": {p: self.sha(f"demo-1.0.0-{p}.tar.gz") for p in PLATFORMS}}
        if signatures:
            row["platform_signatures"] = {p: {"key_id": "fixture", "alg": "ed25519", "sig": f"old-{p}"} for p in PLATFORMS}
        row.update(extra)
        self.releases.write_text(json.dumps({"releases": {"demo": row}}))
        return row

    def row(self):
        return json.loads(self.releases.read_text())["releases"]["demo"]

    def assert_missing_platforms(self, count=5):
        result = self.verify()
        output = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1, output)
        self.assertIn(f"{count} expected asset(s) are genuinely absent", output)
        self.assertEqual(self.releases.read_text(), self.original)

    def test_signatures_without_published_manifest_is_refused(self):
        self.archive_all()
        self.sign_platforms()
        result = self.verify(published=None)
        output = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1, output)
        self.assertIn("--signatures needs --published-sha256", output)
        self.assertEqual(self.releases.read_text(), self.original)

    def test_tarball_changed_since_the_manifest_stops_the_write(self):
        self.archive_all()
        self.sign_platforms()
        for case, overrides in (("platform", {"demo-1.0.0-linux-arm64.tar.gz": "0" * 64}), ("source", {"demo-1.0.0.tar.gz": "0" * 64})):
            with self.subTest(case=case):
                result = self.verify(published=self.approve(overrides))
                output = result.stdout + result.stderr
                self.assertEqual(result.returncode, 1, output)
                self.assertIn("UNAPPROVED demo", output)
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
        self.archive_all()
        self.sign_platforms()
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        row = self.row()
        self.assertEqual(set(row["platform_checksums"]), set(PLATFORMS))
        self.assertEqual(row["binaries"], ["nself-demo"])
        # Each platform carries its own signature object, without the covered sha256 (that is platform_checksums).
        self.assertEqual(row["platform_signatures"], {
            p: {"key_id": "fixture", "alg": "ed25519", "sig": f"fixture-{p}"} for p in PLATFORMS})

    def test_published_platform_without_signature_stops_the_write(self):
        self.archive_all()
        self.sign_platforms()
        for missing in ("absent", "stale", "none"):
            with self.subTest(missing=missing):
                self.sign_platforms()
                doc = json.loads(self.signatures.read_text())
                if missing == "absent":
                    del doc["demo"]["platform_signatures"]["linux-arm64"]
                elif missing == "stale":
                    doc["demo"]["platform_signatures"]["linux-arm64"]["sha256"] = "0" * 64
                else:
                    del doc["demo"]["platform_signatures"]
                self.signatures.write_text(json.dumps(doc))
                result = self.verify()
                output = result.stdout + result.stderr
                self.assertEqual(result.returncode, 1, output)
                self.assertIn("UNSIGNED demo/", output)
                self.assertEqual(self.releases.read_text(), self.original)

    def test_platform_signature_for_another_platforms_bytes_is_refused(self):
        self.archive_all()
        self.sign_platforms()
        doc = json.loads(self.signatures.read_text())
        # darwin-arm64 carries the signed hash of darwin-amd64.
        doc["demo"]["platform_signatures"]["darwin-arm64"]["sha256"] = self.sha("demo-1.0.0-darwin-amd64.tar.gz")
        self.signatures.write_text(json.dumps(doc))
        result = self.verify()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("UNSIGNED demo/darwin-arm64", result.stdout)

    def test_source_only_release_gets_no_platform_signatures(self):
        self.exception(expires="2026-12-31")
        self.sign_platforms(sha="0" * 64)
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("platform_signatures", self.row())

    def test_rewrite_keeps_platform_signatures_when_bytes_are_unchanged(self):
        self.archive_all()
        self.seed_row()
        result = self.verify(signed=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.row()["platform_signatures"], {
            p: {"key_id": "fixture", "alg": "ed25519", "sig": f"old-{p}"} for p in PLATFORMS})

    def test_rewrite_drops_only_the_platform_signature_whose_bytes_changed(self):
        self.archive_all()
        self.seed_row()
        self.archive("demo-1.0.0-linux-arm64.tar.gz", marker="rebuilt")
        result = self.verify(signed=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        kept = self.row()["platform_signatures"]
        self.assertEqual(set(kept), set(PLATFORMS) - {"linux-arm64"})

    def test_rewrite_drops_all_platform_signatures_when_source_bytes_changed(self):
        self.archive_all()
        self.seed_row()
        self.archive("demo-1.0.0.tar.gz", marker="rebuilt")
        result = self.verify(signed=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        row = self.row()
        self.assertIsNone(row["release_signature"])
        self.assertNotIn("platform_signatures", row)

    def test_rewrite_drops_platform_signatures_made_for_another_version(self):
        self.archive_all()
        self.seed_row(version="0.9.0")
        result = self.verify(signed=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("platform_signatures", self.row())

    def test_rewrite_without_platform_signatures_adds_none(self):
        self.archive_all()
        self.seed_row(signatures=False)
        result = self.verify(signed=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("platform_signatures", self.row())

    def test_source_plugin_needs_no_binary_exception(self):
        self.manifest.write_text("{}")
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
