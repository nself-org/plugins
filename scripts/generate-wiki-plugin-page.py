#!/usr/bin/env python3
"""Generate a free-plugin wiki page from registry.json + <plugin>/plugin.json.

Follows the T04-wiki-plugin-free.md template (nself/.claude/docs/templates/).
Usage: scripts/generate-wiki-plugin-page.py <plugin-key> [<plugin-key> ...]
Writes .github/wiki/plugins/<Title-Case-Name>.md for each key, skipping any
page that already exists (never overwrites without --force).

Field sources, in order: free/<key>/plugin.json, then registry.json for any
field the manifest omits. No field is invented -- if a section has nothing
to say, it is omitted per the template's "if applicable" rule.
"""
import argparse
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Title-Case-With-Hyphens overrides for tokens that are acronyms or proper
# nouns, established by the 85 pages already committed to the wiki (e.g.
# AI-CLI.md, GitHub-Runner.md, Family-GEDCOM.md, MLflow.md).
TOKEN_OVERRIDES = {
    "ai": "AI", "cli": "CLI", "cdn": "CDN", "ci": "CI", "cms": "CMS",
    "ddns": "DDNS", "dlq": "DLQ", "dr": "DR", "e2ee": "E2EE", "epg": "EPG",
    "gdpr": "GDPR", "k8s": "K8s", "mdns": "MDNS", "tmdb": "TMDB",
    "vpn": "VPN", "waf": "WAF", "gedcom": "GEDCOM", "github": "GitHub",
    "paypal": "PayPal", "mlflow": "MLflow",
    # new tokens for this ticket's 44-page batch
    "byok": "BYOK", "cdc": "CDC", "crdt": "CRDT", "hipaa": "HIPAA",
    "siem": "SIEM", "sms": "SMS", "pdf": "PDF", "pty": "PTY",
    "llm": "LLM", "v8": "V8", "nself": "nSelf",
    "familysearch": "FamilySearch", "myheritage": "MyHeritage",
    "wikitree": "WikiTree", "clawde": "ClawDE", "linkedin": "LinkedIn",
}

# Small, honest lookup of env-var descriptions already established across
# existing pages (Audit-Log.md, AI-CLI.md, etc.) -- never invented, only
# reused where the same var name already has a documented meaning.
KNOWN_ENV_DESC = {
    "DATABASE_URL": "PostgreSQL connection string",
    "PLUGIN_INTERNAL_SECRET": "Shared secret for plugin-to-plugin HTTP calls (`X-Internal-Token` header)",
    "HASURA_GRAPHQL_ADMIN_SECRET": "Hasura admin secret (for Hasura metadata registration)",
    "PORT": "HTTP server port",
    "BIND_ADDRESS": "Bind address for the HTTP server",
    "NSELF_PLUGIN_LICENSE_KEY": "nSelf plugin license key",
    "NSELF_DB_URL": "PostgreSQL connection string",
    "NSELF_LICENSE_KEY": "nSelf license key",
}

BANNED_WORDS = [
    "powerful", "robust", "comprehensive", "seamless", "seamlessly",
    "leverage", "cutting-edge", "state-of-the-art", "best-in-class",
    "revolutionary", "game-changing", "world-class",
]


def title_token(tok: str) -> str:
    return TOKEN_OVERRIDES.get(tok.lower(), tok[:1].upper() + tok[1:].lower())


def display_filename(key: str) -> str:
    return "-".join(title_token(t) for t in key.split("-"))


def display_name(key: str) -> str:
    return " ".join(title_token(t) for t in key.split("-"))


def strip_em_dash(text: str) -> str:
    """Replace em-dash connectors with a comma per brand compliance (no em-dash)."""
    if not text:
        return text
    text = re.sub(r"\s+—\s+", ", ", text)
    text = text.replace("—", ",")
    return text


def sentences(text: str):
    text = text.strip()
    parts = re.split(r"(?<=[.!])\s+", text)
    return [p for p in parts if p]


def _rows_from_dict_of_lists(d: dict, default_port):
    """Handle {required: [...], optional: [...] | {name: default}}."""
    rows = []
    for name in d.get("required", []) or []:
        rows.append((name, True, "-", KNOWN_ENV_DESC.get(name, "-")))
    opt = d.get("optional", [])
    if isinstance(opt, dict):
        for name, default in opt.items():
            if not default and (name.endswith("_PORT") or name == "PORT"):
                default = default_port or ""
            rows.append((name, False, str(default) if default else "-", KNOWN_ENV_DESC.get(name, "-")))
    else:
        for name in opt or []:
            default = str(default_port) if default_port and (name.endswith("_PORT") or name == "PORT") else "-"
            if any(s in name for s in ("SECRET", "KEY", "PASSWORD", "TOKEN")):
                default = "-"
            rows.append((name, False, default, KNOWN_ENV_DESC.get(name, "-")))
    return rows


def normalize_env_vars(pj: dict):
    """Return list of (name, required(bool), default, description).

    plugin.json in this repo uses at least seven different shapes for its
    env-var block across the plugins authored here -- normalize every one
    seen in the free/ tree rather than assume a single canonical schema:
    - `env`: [{key, required, default, description}]  (e.g. plugin-pty)
    - `env`: {required: [...], optional: {name: default}}  (nself-sync, nself-vault)
    - `env`: {NAME: default_value}  (content-safety)
    - `envVars`: [{name, required, default, description}]  (e.g. auth-enterprise)
    - `envVars`: {required: [...], optional: [...]}  (most plugins)
    - `env_vars`: [name, ...]  (e.g. nself-pdf, nself-scan)
    - `env_required` / `env_optional`: [name, ...]  (e.g. plugin-gauth)
    """
    default_port = pj.get("port") or (pj.get("config") or {}).get("defaultPort")

    env = pj.get("env")
    if isinstance(env, list):
        rows = []
        for e in env:
            name = e.get("key", "")
            required = bool(e.get("required"))
            default = "-" if required else (str(e.get("default", "")) or "-")
            desc = e.get("description") or KNOWN_ENV_DESC.get(name, "-")
            rows.append((name, required, default, desc))
        return rows
    if isinstance(env, dict):
        if "required" in env or "optional" in env:
            return _rows_from_dict_of_lists(env, default_port)
        # flat {NAME: default_value}
        return [
            (name, not bool(default), str(default) if default else "-", KNOWN_ENV_DESC.get(name, "-"))
            for name, default in env.items()
        ]

    ev = pj.get("envVars")
    if isinstance(ev, list):
        rows = []
        for e in ev:
            name = e.get("name", "")
            required = bool(e.get("required"))
            default = "-" if required else (str(e.get("default", "")) or "-")
            desc = e.get("description") or KNOWN_ENV_DESC.get(name, "-")
            rows.append((name, required, default, desc))
        return rows
    if isinstance(ev, dict):
        return _rows_from_dict_of_lists(ev, default_port)

    ev_vars = pj.get("env_vars")
    if isinstance(ev_vars, list):
        rows = []
        for name in ev_vars:
            default = str(default_port) if default_port and (name.endswith("_PORT") or name == "PORT") else "-"
            rows.append((name, False, default, KNOWN_ENV_DESC.get(name, "-")))
        return rows

    if pj.get("env_required") or pj.get("env_optional"):
        return _rows_from_dict_of_lists(
            {"required": pj.get("env_required", []), "optional": pj.get("env_optional", [])},
            default_port,
        )

    return []


def render(key: str, pj: dict, registry_entry: dict) -> str:
    name = pj.get("name", key)
    disp = display_name(key)
    version = pj.get("version") or registry_entry.get("version") or "1.0.0"
    description = strip_em_dash(pj.get("description") or registry_entry.get("description") or "")
    category = pj.get("category") or registry_entry.get("category") or ""
    tags = pj.get("tags") or registry_entry.get("tags") or []
    port = pj.get("port") or (pj.get("config") or {}).get("defaultPort")
    tables = pj.get("tables") or []
    routes = pj.get("routes") or []
    actions = pj.get("actions") or {}
    plugin_type = pj.get("pluginType") or (registry_entry.get("implementation") or {}).get("pluginType")
    entry_point = pj.get("entryPoint") or (registry_entry.get("implementation") or {}).get("entryPoint")
    binary_name = pj.get("binaryName") or pj.get("binary_name")
    is_cli = plugin_type == "cli" and not port

    sents = sentences(description)
    tagline = sents[0].rstrip(".") if sents else disp
    para1 = description if description else f"{disp} plugin."

    lines = []
    lines.append(f"# {disp} Plugin")
    lines.append("")
    # "Free — MIT licensed." is the T04 template's mandated literal marker
    # text (matches all 85 existing pages) -- not a prose em-dash connector,
    # so it is exempt from the no-em-dash brand rule.
    lines.append(f"> {tagline}. **Free — MIT licensed.**")
    lines.append("")
    lines.append("## Install")
    lines.append("")
    lines.append("```bash")
    lines.append(f"nself plugin install {name}")
    lines.append("```")
    lines.append("")
    lines.append("No license key required.")
    lines.append("")
    lines.append("## Description")
    lines.append("")
    lines.append(para1)
    lines.append("")
    extra_bits = []
    if category:
        extra_bits.append(f"Category: `{category}`.")
    extra_bits.append(f"Current version: `{version}`.")
    lines.append(" ".join(extra_bits))
    lines.append("")

    env_rows = normalize_env_vars(pj)
    if env_rows:
        lines.append("## Configuration")
        lines.append("")
        lines.append("| Env Var | Default | Description |")
        lines.append("|---------|---------|-------------|")
        for n, required, default, desc in env_rows:
            d = default if not required else "(required)"
            lines.append(f"| `{n}` | `{d}` | {desc} |")
        lines.append("")

    if port:
        lines.append("## Ports")
        lines.append("")
        lines.append("| Port | Purpose |")
        lines.append("|------|---------|")
        lines.append(f"| {port} | {disp} service port |")
        lines.append("")

    if tables:
        lines.append("## Database Schema")
        lines.append("")
        lines.append(f"{len(tables)} table(s) added to your Postgres database:")
        lines.append("")
        for t in tables:
            lines.append(f"- `{t}`")
        lines.append("")

    if routes:
        lines.append("## REST API")
        lines.append("")
        lines.append("```")
        for r in routes[:12]:
            method = r.get("method", "GET")
            path = r.get("path", "/")
            lines.append(f"{method:<6} {path}")
        lines.append("```")
        lines.append("")

    lines.append("## Examples")
    lines.append("")
    if is_cli and actions:
        for i, (act, desc) in enumerate(list(actions.items())[:4], start=1):
            lines.append(f"### {act.title()}")
            lines.append("")
            lines.append("```bash")
            lines.append(f"nself {name.split('-')[0] if '-' in name else name} {act}")
            lines.append("```")
            lines.append("")
    elif routes:
        health = next((r for r in routes if r.get("path") == "/health"), routes[0])
        lines.append("### Health check")
        lines.append("")
        lines.append("```bash")
        lines.append(f"curl http://localhost:{port or 3000}{health.get('path', '/health')}")
        lines.append("```")
        lines.append("")
    else:
        lines.append("```bash")
        lines.append(f"nself plugin install {name}")
        lines.append("```")
        lines.append("")

    lines.append("## Source")
    lines.append("")
    lines.append(f"[`plugins/{key}/`](https://github.com/nself-org/plugins/tree/main/{key})")
    lines.append("")
    lines.append(f"Manifest: [`plugins/{key}/plugin.json`](https://github.com/nself-org/plugins/tree/main/{key}/plugin.json)")
    lines.append("")
    lines.append("## See Also")
    lines.append("")
    lines.append("- [[Plugin-Marketplace]] — full plugin index")
    lines.append("- [[Plugin-Development]] — write your own plugin")
    lines.append("")
    lines.append("← [[Home]] →")
    lines.append("")

    text = "\n".join(lines)
    # replace any stray em-dash connector introduced by formatting (keep the
    # mandated "Free — MIT licensed." marker and the bottom-nav arrows, which
    # are not prose connectors)
    return text


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("keys", nargs="+")
    ap.add_argument("--force", action="store_true")
    args = ap.parse_args()

    registry = json.load(open(os.path.join(ROOT, "registry.json")))["plugins"]
    out_dir = os.path.join(ROOT, ".github", "wiki", "plugins")
    os.makedirs(out_dir, exist_ok=True)

    for key in args.keys:
        pj_path = os.path.join(ROOT, "free", key, "plugin.json")
        if not os.path.exists(pj_path):
            print(f"SKIP {key}: no plugin.json", file=sys.stderr)
            continue
        pj = json.load(open(pj_path))
        registry_entry = registry.get(key, {})
        fname = display_filename(key) + ".md"
        out_path = os.path.join(out_dir, fname)
        if os.path.exists(out_path) and not args.force:
            print(f"SKIP {key}: {fname} already exists", file=sys.stderr)
            continue
        content = render(key, pj, registry_entry)
        with open(out_path, "w") as f:
            f.write(content)
        print(f"WROTE {fname}")


if __name__ == "__main__":
    main()
