#!/usr/bin/env python3
"""Tests for scripts/generate-wiki-plugin-page.py (P7-TRUTH-19).

unittest.TestCase classes only, so `python3 -m pytest` and `python3 -m unittest`
both collect them. Fixtures live in scripts/testdata/wiki-plugin-pages/.
"""
import contextlib
import importlib.util
import io
import json
import os
import shutil
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
FIXTURES = os.path.join(HERE, "testdata", "wiki-plugin-pages")

_spec = importlib.util.spec_from_file_location("gen", os.path.join(HERE, "generate-wiki-plugin-page.py"))
gen = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(gen)


def ci_manifest(names):
    subs = [{"name": n, "json": "none"} if n == "serve" else {"name": n} for n in names]
    return {"commands": {"command": "ci", "json": "envelope", "output": "document", "side_effect": "read",
                         "subcommands": subs}}


class WikiPagesCase(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp(prefix="wiki-plugin-pages-")
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        shutil.copy(os.path.join(FIXTURES, "catalog.json"), self.root)
        shutil.copytree(os.path.join(FIXTURES, "pages"), os.path.join(self.root, gen.PAGES_DIR))
        self.config = os.path.join(FIXTURES, "wiki-plugin-pages.yaml")
        self.write_manifest(["run", "serve", "nodes provision", "nodes verify"])

    def write_manifest(self, names):
        path = os.path.join(self.root, "free", "ci", "plugin.json")
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(ci_manifest(names), fh)

    def page(self, name):
        with open(os.path.join(self.root, gen.PAGES_DIR, name), encoding="utf-8") as fh:
            return fh.read()

    def cli(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = gen.main(["--root", self.root, "--config", self.config, *args])
        return code, out.getvalue()

    def generate_all(self):
        self.assertEqual(self.cli("--all")[0], 0)


class RegionTests(WikiPagesCase):
    def test_region_insert_leaves_authored_bytes_unchanged(self):
        before = self.page("Jobs.md")
        self.cli("jobs")
        after = self.page("Jobs.md")
        region = after[after.index(gen.BEGIN): after.index(gen.END) + len(gen.END)]
        stripped = before.replace(" **Free — MIT licensed.**", "")
        self.assertEqual(after, "# Jobs Plugin\n\n" + region + "\n" + stripped[len("# Jobs Plugin\n"):])
        # The region sits directly under the H1.
        self.assertTrue(after.startswith("# Jobs Plugin\n\n" + gen.BEGIN))
        # Everything after the old blockquote is byte for byte the authored text.
        tail = before[before.index("## Install"):]
        self.assertTrue(after.endswith(tail))

    def test_old_generator_phrase_removed_only(self):
        self.cli("jobs")
        text = self.page("Jobs.md")
        self.assertNotIn("Free — MIT licensed", text)
        self.assertIn("> Background job queue.", text)

    def test_second_run_is_a_no_op(self):
        self.generate_all()
        snapshot = {n: self.page(n) for n in os.listdir(os.path.join(self.root, gen.PAGES_DIR))}
        code, out = self.cli("--all")
        self.assertEqual(code, 0)
        self.assertEqual(snapshot, {n: self.page(n) for n in snapshot})
        self.assertEqual(self.cli("--all", "--check")[0], 0, out)

    def test_hand_edit_inside_region_is_stale(self):
        self.generate_all()
        path = os.path.join(self.root, gen.PAGES_DIR, "Jobs.md")
        edited = self.page("Jobs.md").replace("1.2.0", "9.9.9")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(edited)
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        self.assertIn("Jobs.md: stale region", out)

    def test_missing_page_is_created_as_h1_plus_region(self):
        self.assertFalse(os.path.exists(os.path.join(self.root, gen.PAGES_DIR, "Voice.md")))
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        self.assertIn("Voice.md: missing page", out)
        self.cli("voice")
        text = self.page("Voice.md")
        self.assertTrue(text.startswith("# Voice Plugin\n\n" + gen.BEGIN))
        self.assertTrue(text.endswith(gen.END + "\n"))

    def test_title_override_picks_the_filename(self):
        self.cli("api")
        self.assertIn("# API Plugin", self.page("API.md"))

    def test_orphan_page_fails_check_but_index_pages_do_not(self):
        self.generate_all()
        with open(os.path.join(self.root, gen.PAGES_DIR, "Gone.md"), "w", encoding="utf-8") as fh:
            fh.write("# Gone Plugin\n")
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        self.assertIn("Gone.md: orphan page", out)
        self.assertNotIn("Index.md", out)

    def test_unknown_slug_is_an_error(self):
        self.assertEqual(self.cli("nope")[0], 2)


class MalformedPageTests(WikiPagesCase):
    def put(self, name, text):
        with open(os.path.join(self.root, gen.PAGES_DIR, name), "w", encoding="utf-8") as fh:
            fh.write(text)

    def test_lone_markers_and_wrong_order_are_reported_not_patched(self):
        cases = {
            "Jobs.md": f"# Jobs Plugin\n\n{gen.BEGIN}\nold\n",
            "Cron.md": f"# Cron Plugin\n\nold\n{gen.END}\n",
            "API.md": f"# API Plugin\n{gen.END}\n{gen.BEGIN}\n",
        }
        for name, text in cases.items():
            self.put(name, text)
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        for name in cases:
            self.assertIn(f"{name}: malformed region markers", out)
        self.assertIn("Voice.md: missing page", out)  # other problems still listed
        before = self.page("Jobs.md")
        self.assertEqual(self.cli("jobs")[0], 1)
        self.assertEqual(self.page("Jobs.md"), before)

    def test_duplicated_region_is_reported(self):
        self.generate_all()
        text = self.page("Jobs.md")
        region = text[text.index(gen.BEGIN): text.index(gen.END) + len(gen.END)]
        self.put("Jobs.md", text + "\n" + region + "\n")
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        self.assertIn("Jobs.md: malformed region markers (2 BEGIN, 2 END", out)

    def test_page_without_h1_names_the_file_and_keeps_going(self):
        self.put("Jobs.md", "intro only\n")
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        self.assertIn("Jobs.md: no H1 line", out)
        self.assertIn("Voice.md: missing page", out)

    def test_blockquote_holding_only_the_old_phrase_is_dropped(self):
        self.put("Jobs.md", "# Jobs Plugin\n\n> **Free — MIT licensed.**\n\nBody.\n")
        self.cli("jobs")
        text = self.page("Jobs.md")
        self.assertNotIn("> ", text.replace("| ", ""))
        self.assertTrue(text.endswith(gen.END + "\n\nBody.\n"))

    def test_missing_ceiling_manifest_is_a_clear_error(self):
        os.remove(os.path.join(self.root, "free", "ci", "plugin.json"))
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 2)


class ContentTests(WikiPagesCase):
    def test_licensed_page_has_bundle_membership_and_no_source_link(self):
        self.cli("voice")
        text = self.page("Voice.md")
        self.assertIn("Licensed, in the Claw Bundle", text)
        self.assertNotIn("http", text.replace(gen.MANIFEST_URL, ""))
        self.assertNotIn("tar.gz", text)
        self.assertNotIn("/download", text)
        self.assertIn("`nself voice`: `say`, `listen`", text)

    def test_free_page_states_licence_version_install_requirements(self):
        self.cli("jobs")
        text = self.page("Jobs.md")
        for want in ("| **Licence** | Free (MIT) |", "| **Version** | 1.2.0 |",
                     "`nself plugin install jobs`", "| **Requirements** | nSelf >=1.0.0 |",
                     "implemented ([maturity levels](" + gen.MANIFEST_URL + "))"):
            self.assertIn(want, text)

    def test_tier_pair_shares_one_page_with_both_tiers(self):
        self.cli("cron")
        text = self.page("Cron.md")
        self.assertIn("Free (MIT) and Licensed, in the Claw Bundle", text)
        self.assertIn("free: 1.2.0; licensed: 2.0.0", text)
        self.assertEqual(text.count(gen.BEGIN), 1)
        self.assertIn("Authored intro.", text)

    def test_docs_link_only_when_the_catalogue_has_one(self):
        self.cli("api", "jobs")
        self.assertIn("[https://docs.example.test/api]", self.page("API.md"))
        self.assertNotIn("**Docs**", self.page("Jobs.md"))


class CeilingTests(WikiPagesCase):
    def check(self):
        self.cli("--all")
        return self.cli("--all", "--check")

    def test_thirteen_first_level_subcommands_fail_and_print_the_count(self):
        self.write_manifest([f"cmd{i}" for i in range(13)])
        code, out = self.check()
        self.assertEqual(code, 1)
        self.assertIn("ci: 13 first-level subcommands (ceiling 12)", out)
        self.assertIn("exceeds the ceiling 12", out)

    def test_twelve_pass(self):
        self.write_manifest([f"cmd{i}" for i in range(12)])
        code, out = self.check()
        self.assertEqual(code, 0, out)
        self.assertIn("ci: 12 first-level subcommands", out)

    def test_aliases_are_not_counted(self):
        names = [f"cmd{i}" for i in range(12)] + ["run", "eval", "build", "forgejo"]
        self.write_manifest(names)
        code, out = self.check()
        self.assertEqual(code, 0, out)
        self.assertIn("ci: 12 first-level subcommands", out)

    def test_only_the_first_token_counts(self):
        self.write_manifest([f"nodes sub{i}" for i in range(20)])
        self.assertIn("ci: 1 first-level subcommands", self.check()[1])


class MachineSurfaceTests(WikiPagesCase):
    def vectors(self):
        with open(os.path.join(FIXTURES, "surface-vectors.json"), encoding="utf-8") as fh:
            return json.load(fh)

    def test_naming_vectors(self):
        for v in self.vectors()["names"]:
            self.assertEqual(gen.tool_name(v["path"]), v["tool"])
            self.assertEqual(gen.route_path(v["path"]), v["route"])

    def test_exposure_vectors(self):
        for v in self.vectors()["exposure"]:
            self.assertEqual(gen.exposure(v["cmd"]), (v["exposure"], v["reason"], v["confirmation"]), v["name"])

    def test_region_follows_catalog_region_with_a_row_per_command(self):
        self.generate_all()
        text = self.page("CI.md")
        self.assertIn(gen.MS_BEGIN, text)
        self.assertLess(text.index(gen.END), text.index(gen.MS_BEGIN))
        self.assertIn("| `nself ci nodes provision` | `nself_ci_nodes_provision` | `/v1/commands/ci/nodes/provision` |", text)
        self.assertIn("| `nself ci serve` | - | - | read | document | CLI only (json none) | none |", text)

    def test_bare_hub_command_has_its_own_row(self):
        """`nself ci` (tool nself_ci) is a command too: a manifest with subcommands still gets the hub's own row."""
        self.generate_all()
        text = self.page("CI.md")
        self.assertIn("| `nself ci` | `nself_ci` | `/v1/commands/ci` | read | document | MCP and HTTP | none |", text)
        rows = [r for r in text.splitlines() if r.startswith("| `nself ci")]
        self.assertEqual(rows[0].split("|")[1].strip(), "`nself ci`", "the bare command sorts before its subcommands")

    def test_bare_hub_row_carries_its_confirmation(self):
        manifest = ci_manifest(["run"])
        manifest["commands"].update({"side_effect": "remote", "confirm": {"flags": []}})
        rows = dict(gen.machine_rows(manifest))
        self.assertEqual(sorted(rows), ["ci", "ci run"])
        self.assertEqual(gen.exposure(rows["ci"]), ("MCP and HTTP", "", "nonce"))

    def test_stream_read_row_is_http_only(self):
        self.write_manifest(["run"])
        path = os.path.join(self.root, "free", "ci", "plugin.json")
        with open(path, "w", encoding="utf-8") as fh:
            json.dump({"commands": {"command": "ci", "json": "envelope", "output": "stream", "side_effect": "read",
                                    "subcommands": [{"name": "run"}]}}, fh)
        self.generate_all()
        self.assertIn("| `nself ci run` | - | `/v1/commands/ci/run` | read | stream | HTTP NDJSON only (stream) | none |",
                      self.page("CI.md"))

    def test_plugin_without_commands_has_no_region_and_bytes_outside_are_kept(self):
        self.generate_all()
        self.assertNotIn(gen.MS_BEGIN, self.page("Jobs.md"))
        text = self.page("CI.md")
        extended = text + "\nAuthored tail.\n"
        self.assertEqual(gen.apply_machine(extended, None).count(gen.MS_BEGIN), 0)
        self.assertTrue(gen.apply_machine(extended, None).endswith("\nAuthored tail.\n"))
        self.assertEqual(gen.apply_machine(gen.apply_machine(extended, None), None), gen.apply_machine(extended, None))

    def test_manifest_change_makes_check_fail_naming_the_page(self):
        self.generate_all()
        self.write_manifest(["run", "serve", "nodes provision", "nodes verify", "extra"])
        code, out = self.cli("--all", "--check")
        self.assertEqual(code, 1)
        self.assertIn("CI.md: stale region", out)

    def test_second_run_is_a_no_op(self):
        self.generate_all()
        before = self.page("CI.md")
        self.generate_all()
        self.assertEqual(before, self.page("CI.md"))


if __name__ == "__main__":
    unittest.main()
