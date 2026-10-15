"""D-0628: socket scans preserve filenames; Hasura tables have separate files."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]
RULE = ROOT / "scripts/conformance/rules/docker-socket.sh"
TABLES = {
    "media-processing": (
        "mediap", "encoding_profiles jobs job_outputs hls_manifests subtitles "
        "trickplay webhook_events watcher_events uploads",
    ),
    "podcast": (
        "podcast", "podcasts episodes subscriptions playback_positions categories",
    ),
}


class DockerSocketTests(unittest.TestCase):
    def test_socket_mount_in_unusual_filename(self):
        for filename, named in (("extra/one\ntwo.yaml", False),
                                ("extra/one\ntwo.yml", False),
                                ("Dockerfile\none", False),
                                ("deploy/one\ntwo.fragment", True),
                                ("extra/with spaces.yaml", False)):
            for mount in ("/var/run/docker.sock:/s", "/var/run:/host-run"):
                with self.subTest(filename=filename, mount=mount):
                    with tempfile.TemporaryDirectory() as tmp:
                        plugin = Path(tmp) / "sample"
                        plugin.mkdir()
                        manifest = {"name": "sample"}
                        if named:
                            manifest["service"] = {"compose": filename}
                        (plugin / "plugin.json").write_text(json.dumps(manifest))
                        target = plugin / filename
                        target.parent.mkdir(parents=True, exist_ok=True)
                        target.write_text("volumes:\n  - " + mount + "\n")
                        exceptions = Path(tmp) / "exceptions.yaml"
                        exceptions.write_text("[]\n")
                        result = subprocess.run(
                            ["bash", str(RULE), str(plugin), str(exceptions)],
                            capture_output=True, text=True, check=False,
                        )
                        self.assertEqual(result.returncode, 1, result.stderr)
                        self.assertIn("FAIL docker-socket sample:", result.stdout)
                        self.assertIn(filename, result.stdout)
                        target.write_text("volumes: []\n")
                        clean = subprocess.run(
                            ["bash", str(RULE), str(plugin), str(exceptions)],
                            capture_output=True, text=True, check=False,
                        )
                        self.assertEqual(clean.returncode, 0, clean.stdout + clean.stderr)


class HasuraMetadataTests(unittest.TestCase):
    def test_table_files_and_includes(self):
        for plugin, (prefix, names) in TABLES.items():
            with self.subTest(plugin=plugin):
                directory = ROOT / "free" / plugin / "hasura/metadata/databases/default/tables"
                tables = [f"np_{prefix}_{name}" for name in names.split()]
                expected = [f"{table}.yaml" for table in tables]
                self.assertEqual(sorted(os.listdir(directory)), sorted(expected + ["tables.yaml"]))
                self.assertEqual(
                    (directory / "tables.yaml").read_text().splitlines(),
                    [f'- "!include {filename}"' for filename in expected],
                )
                for table in tables:
                    text = (directory / f"{table}.yaml").read_text()
                    self.assertTrue(text.startswith(f"- table:\n    schema: public\n    name: {table}\n"))
                    self.assertIn("source_account_id:", text)
                    self.assertIn("_eq: X-Hasura-Source-Account-Id", text)


if __name__ == "__main__":
    unittest.main()
