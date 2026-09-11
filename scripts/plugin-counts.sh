#!/usr/bin/env bash
# Purpose: the SOLE generator of plugin counts. Every consumer (website, CLI,
# docs, marketing, SPORT) had independently reimplemented this rule and each
# got it wrong differently — the free/pro split, the installable rule, and
# the free/pro overlap rule had all drifted apart. This script is the one
# place those rules live; everyone else reads plugins/counts.json instead of
# recomputing anything.
#
# Inputs:
#   registry.json in this repo (free) and in a sibling ../plugins-pro
#   checkout (pro, private). Each entry's own manifest
#   (free|paid/<slug>/plugin.{json,yaml,yml}) supplies the installable flag;
#   the registry entry itself is not the source of truth for that flag
#   (though see the emitted "contradictions" note if one is found anyway).
#   A slug present in both registries is one plugin (a duplicate) unless
#   either registry's entry sets "dual_registry": true, in which case it is
#   two distinct plugins that happen to share a name.
#
# Outputs:
#   Default: a human-readable table on stdout.
#   --json:  the locked schema (see plugins/counts.json) on stdout.
#
# Constraints:
#   --origin reads both registries (and every manifest referenced by them)
#   from origin/main via `git show`, not the working tree — use this for any
#   doc generation or CI, since a local plugins-pro checkout can be on a
#   feature branch. Missing/unreadable ../plugins-pro is a hard failure
#   (never emits free-only numbers — that silent-drift failure mode is
#   exactly what this generator exists to eliminate).
set -euo pipefail

use_origin=0
emit_json=0
for arg in "$@"; do
  case "$arg" in
  --origin) use_origin=1 ;;
  --json) emit_json=1 ;;
  -h | --help)
    echo "Usage: $(basename "$0") [--json] [--origin]"
    echo "  --json     emit the locked counts.json schema instead of a table"
    echo "  --origin   read both registries + manifests from origin/main"
    echo "             instead of each repo's local working tree"
    exit 0
    ;;
  esac
done

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)" # this (free) repo root
siblings="$(dirname "$root")"
pro_repo="$siblings/plugins-pro"

if [ ! -d "$pro_repo/.git" ] && [ ! -f "$pro_repo/.git" ]; then
  echo "ERROR: missing sibling repo: $pro_repo" >&2
  echo "This generator reads pro counts from a sibling checkout of the" >&2
  echo "private nself-org/plugins-pro repo. Clone it next to this repo." >&2
  echo "Refusing to emit free-only counts: a silently-wrong count is the" >&2
  echo "exact failure this generator exists to prevent." >&2
  exit 1
fi

if [ "$use_origin" -eq 1 ]; then
  git -C "$root" fetch origin -q
  git -C "$pro_repo" fetch origin -q
fi

python3 - "$root" "$pro_repo" "$use_origin" "$emit_json" <<'PYEOF'
import json
import subprocess
import sys

free_repo, pro_repo, use_origin, emit_json = sys.argv[1], sys.argv[2], sys.argv[3] == "1", sys.argv[4] == "1"


def git_show(repo, path):
    """Read a file's content from origin/main in `repo`, or None if absent."""
    exists = subprocess.run(
        ["git", "-C", repo, "cat-file", "-e", f"origin/main:{path}"],
        capture_output=True,
    )
    if exists.returncode != 0:
        return None
    out = subprocess.run(
        ["git", "-C", repo, "show", f"origin/main:{path}"],
        capture_output=True, text=True, check=True,
    )
    return out.stdout


def blob_sha(repo, path):
    if use_origin:
        out = subprocess.run(
            ["git", "-C", repo, "rev-parse", f"origin/main:{path}"],
            capture_output=True, text=True, check=True,
        )
        return out.stdout.strip()
    out = subprocess.run(
        ["git", "-C", repo, "hash-object", f"{repo}/{path}"],
        capture_output=True, text=True, check=True,
    )
    return out.stdout.strip()


def read_text(repo, path):
    if use_origin:
        return git_show(repo, path)
    try:
        with open(f"{repo}/{path}") as f:
            return f.read()
    except FileNotFoundError:
        return None


def manifest_installable(repo, tier_dir, slug):
    """True unless the plugin's own manifest declares installable: false.
    Missing manifest => installable (per the locked schema's stated default).
    """
    for name in ("plugin.json", "plugin.yaml", "plugin.yml"):
        text = read_text(repo, f"{tier_dir}/{slug}/{name}")
        if text is None:
            continue
        if name == "plugin.json":
            try:
                data = json.loads(text)
            except json.JSONDecodeError:
                continue
            if data.get("installable") is False:
                return True, False
            return True, True
        # Minimal YAML handling: these manifests are flat key: value files,
        # so a top-level "installable:" line is all we need — no YAML
        # dependency for one boolean field.
        for line in text.splitlines():
            stripped = line.strip()
            if stripped.startswith("installable:"):
                value = stripped.split(":", 1)[1].strip().lower()
                return True, value != "false"
        return True, True
    return False, True  # no manifest found at all


def load_registry(repo, path):
    text = read_text(repo, path)
    if text is None:
        print(f"ERROR: missing {path} in {repo} (origin={use_origin})", file=sys.stderr)
        sys.exit(1)
    return json.loads(text)


free_reg = load_registry(free_repo, "registry.json")
pro_reg = load_registry(pro_repo, "registry.json")
free_plugins = free_reg.get("plugins", {})
pro_plugins = pro_reg.get("plugins", {})

contradictions = []


def tier_report(repo, tier_dir, plugins):
    non_installable = []
    for slug, entry in plugins.items():
        found, installable = manifest_installable(repo, tier_dir, slug)
        if isinstance(entry, dict) and "installable" in entry:
            contradictions.append(
                f"{repo}:{slug} — registry entry carries an installable flag "
                f"directly (installable={entry['installable']}); the locked "
                f"schema says the registry entry never carries this flag. "
                f"The manifest is used as the source of truth regardless."
            )
        if not installable:
            non_installable.append(slug)
    non_installable.sort()
    return {
        "entries": len(plugins),
        "installable": len(plugins) - len(non_installable),
        "nonInstallable": non_installable,
    }, {s: (s not in non_installable) for s in plugins}


free_report, free_status = tier_report(free_repo, "free", free_plugins)
pro_report, pro_status = tier_report(pro_repo, "paid", pro_plugins)

shared = sorted(set(free_plugins) & set(pro_plugins))
dual_registry = sorted(
    s for s in shared
    if bool(free_plugins.get(s, {}).get("dual_registry"))
    or bool(pro_plugins.get(s, {}).get("dual_registry"))
)
duplicates = sorted(s for s in shared if s not in dual_registry)

total_entries = free_report["entries"] + pro_report["entries"] - len(duplicates)
non_dup_free_installable = sum(1 for s in free_plugins if s not in duplicates and free_status[s])
non_dup_pro_installable = sum(1 for s in pro_plugins if s not in duplicates and pro_status[s])
dup_installable = sum(1 for s in duplicates if free_status[s] or pro_status[s])
total_installable = non_dup_free_installable + non_dup_pro_installable + dup_installable

result = {
    "_generated": "GENERATED BY plugins/scripts/plugin-counts.sh — DO NOT HAND EDIT",
    "schema_version": 1,
    "generated_at": subprocess.run(
        ["date", "-u", "+%Y-%m-%dT%H:%M:%SZ"], capture_output=True, text=True, check=True
    ).stdout.strip(),
    "sources": {
        "free": {"repo": "nself-org/plugins", "sha": blob_sha(free_repo, "registry.json")},
        "pro": {"repo": "nself-org/plugins-pro", "sha": blob_sha(pro_repo, "registry.json")},
    },
    "free": free_report,
    "pro": pro_report,
    "overlap": {"sharedSlugs": shared, "dualRegistry": dual_registry, "duplicates": duplicates},
    "totals": {"entries": total_entries, "installable": total_installable},
    "advertised": total_installable,
}

if emit_json:
    print(json.dumps(result, indent=2))
else:
    print(f"free   {free_report['entries']:>4}  installable {free_report['installable']:>4}"
          + (f"  (non-installable: {', '.join(free_report['nonInstallable'])})" if free_report["nonInstallable"] else ""))
    print(f"pro    {pro_report['entries']:>4}  installable {pro_report['installable']:>4}"
          + (f"  (non-installable: {', '.join(pro_report['nonInstallable'])})" if pro_report["nonInstallable"] else ""))
    if shared:
        print(f"       shared slugs: {', '.join(shared)}"
              + (f" (dual-registry, counted twice: {', '.join(dual_registry)})" if dual_registry else "")
              + (f" (duplicate, counted once: {', '.join(duplicates)})" if duplicates else ""))
    print(f"total  {result['totals']['entries']:>4}  installable {result['totals']['installable']:>4}")
    print(f"advertised: {result['advertised']}")

for c in contradictions:
    print(f"NOTE (data contradicts locked schema, not corrected here): {c}", file=sys.stderr)

sys.exit(0)
PYEOF
