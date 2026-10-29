"""Machine-surface and surface-ceiling helpers of generate-wiki-plugin-page.py.

Purpose
    The machine-surface region (P7-CI-62) and the first-level subcommand ceiling
    (D11) of a plugin manifest, split out of the generator to keep both files
    under the 300-line budget. The generator imports every name it uses from here.
Inputs and outputs
    Manifest dicts and page text in; region text, counts and Problem errors out.
    Python 3 standard library only.
"""
import json
import os

BEGIN = "<!-- BEGIN GENERATED:catalog -->"
END = "<!-- END GENERATED:catalog -->"
MS_BEGIN = "<!-- BEGIN GENERATED:machine-surface -->"
MS_END = "<!-- END GENERATED:machine-surface -->"
MS_NOTE = "<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->"


class Problem(Exception):
    """A problem found by --check (or a page that cannot be edited)."""


def _sub_names(commands):
    names = []
    for sub in (commands or {}).get("subcommands") or []:
        names.append(sub["name"] if isinstance(sub, dict) else str(sub))
    return names


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
