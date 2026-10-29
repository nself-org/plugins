#!/usr/bin/env python3
"""Maintain one generated catalog region on every plugin wiki home page.

Reads catalog.json (the plugin catalogue, contract plugins.catalog v1) and, for
every slug, owns only this region directly under the H1 of
.github/wiki/plugins/<Title>.md:

    <!-- BEGIN GENERATED:catalog -->
    ...
    <!-- END GENERATED:catalog -->

Plugins whose manifest has a `commands` block also get a second region directly
after the catalog region:

    <!-- BEGIN GENERATED:machine-surface -->
    ...
    <!-- END GENERATED:machine-surface -->

one row per command path: MCP tool name, HTTP route, side effect, output,
exposure with its reason, and the confirmation kind. The rules mirror the cli
(cmdregistry.ToolName/RoutePath; SURF D2/D5/D15 exposure) as data plus
scripts/testdata/wiki-plugin-pages/surface-vectors.json; the cli is never
imported. A plugin without commands has no such region.

Authored text outside the regions is never touched, with one exception: the old
generator's fixed "**Free — MIT licensed.**" phrase is removed. A missing page
is created as H1 + region. A tier pair (the same slug in the free and licensed
tiers) shares one page.

Usage:
  scripts/generate-wiki-plugin-page.py --all            write every page
  scripts/generate-wiki-plugin-page.py --all --check    fail on a stale region,
                                                        a missing page, an orphan
                                                        page or a surface ceiling
  scripts/generate-wiki-plugin-page.py <slug> [<slug>]  write just those pages

Config: scripts/wiki-plugin-pages.yaml (title overrides, index_pages, surface
ceilings). Python 3 standard library plus PyYAML. Output is deterministic.
"""
import argparse
import json
import os
import re
import sys

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BEGIN = "<!-- BEGIN GENERATED:catalog -->"
END = "<!-- END GENERATED:catalog -->"
MS_BEGIN = "<!-- BEGIN GENERATED:machine-surface -->"
MS_END = "<!-- END GENERATED:machine-surface -->"
MS_NOTE = "<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->"
NOTE = "<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->"
OLD_PHRASE = re.compile(r" ?\*\*Free — MIT licensed\.\*\*")
MANIFEST_URL = "https://github.com/nself-org/cli/wiki/Plugin-Manifest"
PAGES_DIR = os.path.join(".github", "wiki", "plugins")

# Title-Case-With-Hyphens tokens that are acronyms or proper nouns, matching
# the pages already committed (AI-CLI.md, GitHub-Runner.md, Family-GEDCOM.md).
TOKENS = {
    "ai": "AI", "cli": "CLI", "cdn": "CDN", "ci": "CI", "cms": "CMS",
    "ddns": "DDNS", "dlq": "DLQ", "dr": "DR", "e2ee": "E2EE", "epg": "EPG",
    "gdpr": "GDPR", "k8s": "K8s", "mdns": "MDNS", "tmdb": "TMDB",
    "vpn": "VPN", "waf": "WAF", "gedcom": "GEDCOM", "github": "GitHub",
    "paypal": "PayPal", "mlflow": "MLflow", "byok": "BYOK", "cdc": "CDC",
    "crdt": "CRDT", "hipaa": "HIPAA", "siem": "SIEM", "sms": "SMS",
    "pdf": "PDF", "pty": "PTY", "llm": "LLM", "v8": "V8", "nself": "nSelf",
    "familysearch": "FamilySearch", "myheritage": "MyHeritage",
    "wikitree": "WikiTree", "clawde": "ClawDE", "linkedin": "LinkedIn",
    "csam": "CSAM", "rum": "RUM", "slo": "SLO", "saas": "SaaS", "mcp": "MCP",
}


class Problem(Exception):
    """A problem found by --check (or a page that cannot be edited)."""


def load_config(path):
    cfg = {}
    if os.path.exists(path):
        with open(path, encoding="utf-8") as fh:
            cfg = yaml.safe_load(fh) or {}
    cfg.setdefault("titles", {})
    cfg.setdefault("index_pages", [])
    cfg.setdefault("surface_ceilings", {})
    return cfg


def page_title(slug, cfg):
    over = cfg["titles"].get(slug)
    if over:
        return over
    return "-".join(TOKENS.get(t.lower(), t[:1].upper() + t[1:].lower()) for t in slug.split("-"))


def group_by_slug(catalog):
    """slug -> entries, free tier first (a tier pair shares one page)."""
    out = {}
    for entry in catalog["plugins"]:
        out.setdefault(entry["slug"], []).append(entry)
    for entries in out.values():
        entries.sort(key=lambda e: (e["tier"] != "free", e["tier"]))
    return out


def _requires(entry):
    req = entry.get("requires") or {}
    parts = []
    for key in sorted(req):
        val = req[key]
        if isinstance(val, (list, tuple)):
            val = ", ".join(str(v) for v in val)
        parts.append(f"{'nSelf' if key == 'nself' else key.replace('_', ' ')} {val}")
    return "; ".join(parts)


def _sub_names(commands):
    names = []
    for sub in (commands or {}).get("subcommands") or []:
        names.append(sub["name"] if isinstance(sub, dict) else str(sub))
    return names


def _licence(entry, bundles):
    if entry["tier"] == "free":
        return "Free (MIT)"
    shown = [bundles[b]["display"] if b in bundles else b for b in entry.get("bundles") or []]
    return "Licensed, in the " + ", ".join(shown) if shown else "Licensed"


def render_region(entries, catalog):
    """The generated region for one slug; no source links for licensed entries."""
    bundles = catalog.get("bundles") or {}
    slug = entries[0]["slug"]
    pair = len(entries) > 1

    def per_tier(fn):
        vals = [fn(e) for e in entries]
        if not pair or len(set(vals)) == 1:
            return vals[0]
        return "; ".join(f"{e['tier']}: {v}" for e, v in zip(entries, vals))

    rows = [
        ("Licence", " and ".join(_licence(e, bundles) for e in entries)),
        ("Maturity", per_tier(lambda e: e["maturity"]) + f" ([maturity levels]({MANIFEST_URL}))"),
        ("Version", per_tier(lambda e: e["version"])),
        ("Install", f"`nself plugin install {slug}`"),
    ]
    for entry in entries:
        cmds = entry.get("commands")
        if cmds:
            subs = _sub_names(cmds)
            text = f"`nself {cmds['command']}`"
            if subs:
                text += ": " + ", ".join(f"`{s}`" for s in subs)
            rows.append(("Commands", text))
            break
    req = per_tier(_requires)
    if req:
        rows.append(("Requirements", req))
    docs = next((e["docs_url"] for e in entries if e.get("docs_url")), None)
    if docs:
        rows.append(("Docs", f"[{docs}]({docs})"))
    lines = [BEGIN, NOTE, "", "| Field | Value |", "|---|---|"]
    lines += [f"| **{k}** | {v} |" for k, v in rows]
    lines += ["", END]
    return "\n".join(lines)


def tool_name(path):
    """MCP tool name: `nself_` + path tokens joined by `_`, `-` to `_` (cmdregistry.ToolName)."""
    return "nself_" + "_".join(path.split()).replace("-", "_")


def route_path(path):
    """HTTP route: `/v1/commands/` + path tokens joined by `/` (cmdregistry.RoutePath)."""
    return "/v1/commands/" + "/".join(path.split())


def exposure(cmd):
    """(exposure, reason, confirmation) for a command dict with inherited fields (SURF D2, D3, D15)."""
    side, conf = cmd.get("side_effect", "read"), cmd.get("confirm")
    gated = side in ("remote", "destructive")
    kind = ("plan" if conf.get("plan") else "nonce") if gated and conf else "none"
    if cmd.get("surface") == "cli-only":
        return "CLI only", "surface cli-only", "none"
    if cmd.get("json") != "envelope":
        return "CLI only", "json " + str(cmd.get("json")), "none"
    out = cmd.get("output", "document")
    if out == "interactive":
        return "CLI only", "interactive", "none"
    if out == "stream":
        if side == "read":
            return "HTTP NDJSON only", "stream", "none"
        return "CLI only", "stream, not read", "none"
    if gated and not conf:
        return "CLI only", side + " without confirm", "none"
    return "MCP and HTTP", "", kind


def machine_rows(manifest):
    """Rows (path, cmd) of a manifest `commands` block; subcommands inherit the hub's fields."""
    top = manifest.get("commands") or {}
    subs = [s for s in top.get("subcommands") or [] if isinstance(s, dict)]
    keys = ("json", "output", "side_effect", "surface", "confirm")
    base = {k: top[k] for k in keys if k in top}
    rows = [(f"{top['command']} {s['name']}", {**base, **{k: s[k] for k in keys if k in s}}) for s in subs]
    if not rows and top.get("command"):
        rows = [(top["command"], base)]
    return sorted(rows, key=lambda r: r[0])


def render_machine_surface(manifest):
    lines = [MS_BEGIN, MS_NOTE, "", "| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |",
             "|---|---|---|---|---|---|---|"]
    for path, cmd in machine_rows(manifest):
        how, why, conf = exposure(cmd)
        tool = f"`{tool_name(path)}`" if how == "MCP and HTTP" else "-"
        route = f"`{route_path(path)}`" if how != "CLI only" else "-"
        shown = how + (f" ({why})" if why else "")
        lines.append(f"| `nself {path}` | {tool} | {route} | {cmd.get('side_effect', 'read')} | "
                     f"{cmd.get('output', 'document')} | {shown} | {conf} |")
    return "\n".join(lines + ["", MS_END])


def load_manifest(root, entries):
    """Manifest of the first free entry with a commands block, else None (licensed manifests are not in this repo)."""
    for e in entries:
        if e.get("commands") and e["tier"] == "free":
            rel = os.path.join(e["tier"], e["slug"], "plugin.json")
            try:
                with open(os.path.join(root, rel), encoding="utf-8") as fh:
                    return json.load(fh)
            except (OSError, ValueError) as exc:
                raise Problem(f"machine-surface manifest {rel}: {exc}")
    return None


def apply_machine(text, region):
    """Put `region` (or None) right after the catalog region; replace or drop an existing one."""
    nb, ne = text.count(MS_BEGIN), text.count(MS_END)
    if nb or ne:
        if nb != 1 or ne != 1 or text.index(MS_BEGIN) > text.index(MS_END):
            raise Problem(f"malformed machine-surface markers ({nb} BEGIN, {ne} END; need one BEGIN then one END)")
        head, rest = text.split(MS_BEGIN, 1)
        tail = rest.split(MS_END, 1)[1]
        if region is None:
            return head.rstrip("\n") + "\n" + tail.lstrip("\n") if tail.strip() else head.rstrip("\n") + "\n"
        return head + region + tail
    if region is None:
        return text
    head, tail = text.split(END, 1)
    return head + END + "\n\n" + region + tail


def apply_region(text, region):
    """Insert or replace the region under the H1; drop the old fixed phrase."""
    out, dropped = [], False
    for ln in text.split("\n"):
        new = OLD_PHRASE.sub("", ln) if ln.startswith(">") else ln
        if new != ln and new.strip() in ("", ">"):
            dropped = True
            continue
        if dropped and not ln.strip() and out and not out[-1].strip():
            dropped = False  # do not leave a double blank line where the phrase was
            continue
        dropped = False
        out.append(new)
    text = "\n".join(out)
    nb, ne = text.count(BEGIN), text.count(END)
    if nb or ne:
        if nb != 1 or ne != 1 or text.index(BEGIN) > text.index(END):
            raise Problem(f"malformed region markers ({nb} BEGIN, {ne} END; need one BEGIN then one END)")
        head, rest = text.split(BEGIN, 1)
        return head + region + rest.split(END, 1)[1]
    lines = text.split("\n")
    for i, ln in enumerate(lines):
        if ln.startswith("# "):
            rest = lines[i + 1:]
            if rest and rest[0].strip():
                rest.insert(0, "")
            return "\n".join(lines[: i + 1] + [""] + region.split("\n") + rest)
    raise Problem("no H1 line to put the region under")


def new_page(slug, region, cfg):
    return f"# {page_title(slug, cfg).replace('-', ' ')} Plugin\n\n{region}\n"


def surface_count(root, rule):
    """First-level subcommand count of a manifest, aliases and hidden internal commands excluded."""
    try:
        with open(os.path.join(root, rule["manifest"]), encoding="utf-8") as fh:
            manifest = json.load(fh)
    except (OSError, ValueError) as exc:
        raise Problem(f"surface ceiling manifest {rule['manifest']}: {exc}")
    skip = set((rule.get("aliases") or {}).get("names") or [])
    skip |= set((rule.get("internal") or {}).get("names") or [])
    firsts = {n.split()[0] for n in _sub_names(manifest.get("commands")) if n.split()}
    return len(firsts - skip)


def check_ceilings(root, cfg):
    problems, notes = [], []
    for name, rule in sorted(cfg["surface_ceilings"].items()):
        count = surface_count(root, rule)
        notes.append(f"{name}: {count} first-level subcommands (ceiling {rule['max']})")
        if count > rule["max"]:
            problems.append(f"{name}: {count} first-level subcommands exceeds the ceiling {rule['max']}")
    return problems, notes


def plan(root, cfg, catalog, slugs):
    """Yield (slug, path, wanted_text_or_Problem, current_text_or_None)."""
    groups = group_by_slug(catalog)
    for slug in slugs:
        if slug not in groups:
            raise Problem(f"{slug}: not a catalogue slug")
        path = os.path.join(root, PAGES_DIR, page_title(slug, cfg) + ".md")
        region = render_region(groups[slug], catalog)
        try:
            manifest = load_manifest(root, groups[slug])
        except Problem as exc:
            yield slug, path, exc, None
            continue
        mregion = render_machine_surface(manifest) if manifest else None
        if os.path.exists(path):
            with open(path, encoding="utf-8") as fh:
                cur = fh.read()
            try:
                yield slug, path, apply_machine(apply_region(cur, region), mregion), cur
            except Problem as exc:
                yield slug, path, exc, cur
        else:
            yield slug, path, apply_machine(new_page(slug, region, cfg), mregion), None


def orphans(root, cfg, catalog):
    known = {page_title(s, cfg) + ".md" for s in group_by_slug(catalog)} | set(cfg["index_pages"])
    pages = os.path.join(root, PAGES_DIR)
    return sorted(f for f in os.listdir(pages) if f.endswith(".md") and f not in known)


def run(root, cfg_path, slugs, do_all, check):
    cfg = load_config(cfg_path)
    with open(os.path.join(root, "catalog.json"), encoding="utf-8") as fh:
        catalog = json.load(fh)
    if do_all:
        slugs = sorted(group_by_slug(catalog))
    problems, written, failed = [], 0, False
    for slug, path, want, cur in plan(root, cfg, catalog, slugs):
        rel = os.path.relpath(path, root)
        if isinstance(want, Problem):
            problems.append(f"{rel}: {want}")
            failed = True
            continue
        if want == cur:
            continue
        if check:
            problems.append(f"{rel}: " + ("missing page" if cur is None else "stale region"))
        else:
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w", encoding="utf-8", newline="\n") as fh:
                fh.write(want)
            written += 1
    if do_all:
        problems += [f"{PAGES_DIR}/{f}: orphan page (not a catalogue slug or index page)" for f in orphans(root, cfg, catalog)]
        found, notes = check_ceilings(root, cfg)
        problems += found
        failed = failed or bool(found)
        for note in notes:
            print(note)
    if check:
        for p in problems:
            print("FAIL " + p)
        print(f"wiki-plugin-pages: {len(problems)} problem(s) in {len(slugs)} slug(s)")
        return 1 if problems else 0
    for p in problems:
        print("note: " + p)
    print(f"wiki-plugin-pages: wrote {written} of {len(slugs)} page(s)")
    return 1 if failed else 0


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("slugs", nargs="*", help="catalogue slugs (omit with --all)")
    ap.add_argument("--all", action="store_true", help="every catalogue slug plus the orphan scan")
    ap.add_argument("--check", action="store_true", help="write nothing; exit 1 on drift")
    ap.add_argument("--root", default=ROOT, help="repo root (default: this repo)")
    ap.add_argument("--config", help="config file (default: scripts/wiki-plugin-pages.yaml under --root)")
    args = ap.parse_args(argv)
    if not args.all and not args.slugs:
        ap.error("give slugs or --all")
    cfg = args.config or os.path.join(args.root, "scripts", "wiki-plugin-pages.yaml")
    try:
        return run(args.root, cfg, args.slugs, args.all, args.check)
    except Problem as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
