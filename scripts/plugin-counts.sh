#!/usr/bin/env bash
# Purpose: the SOLE generator of plugin counts. Every consumer (website, CLI,
# docs, marketing, SPORT) had independently reimplemented this rule and each
# got it wrong differently — the free/pro split, the installable rule, and
# the free/pro overlap rule had all drifted apart. This script is the one
# place those rules live; everyone else reads plugins/counts.json instead of
# recomputing anything.
#
# Inputs:
#   registry.json in this repo (free) and in a sibling ../bundles
#   checkout (pro, private). Each entry's own manifest
#   (free|paid/<slug>/plugin.{json,yaml,yml}) supplies the installable flag.
#   A registry entry may mirror that flag, but the manifest wins; if the two
#   disagree the script prints a DRIFT line on stderr and still counts by the
#   manifest.
#   A slug present in both registries is one two-tier product (the free entry
#   is its free tier, the pro entry its pro tier) and counts once. Both sides
#   must carry "tier_pair": true; an unflagged shared slug is a hard error,
#   not a second way of counting.
#
# Outputs:
#   Default: a human-readable table on stdout.
#   --json:  the locked schema (see plugins/counts.json) on stdout.
#
# Constraints:
#   --origin reads both registries (and every manifest referenced by them)
#   from origin/main via `git show`, not the working tree — use this for any
#   doc generation or CI, since a local bundles checkout can be on a
#   feature branch. Missing/unreadable ../bundles is a hard failure
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
# The licensed-plugin repo was renamed nself-org/plugins-pro -> nself-org/bundles
# (ADR-P6-01). Accept either sibling directory name: a clone made after the
# rename is "bundles", one made before it is still "plugins-pro" on disk and
# reaches GitHub through the rename redirect. Prefer the new name.
pro_repo="$siblings/bundles"
if [ ! -e "$pro_repo/.git" ] && [ -e "$siblings/plugins-pro/.git" ]; then
  pro_repo="$siblings/plugins-pro"
fi

if [ ! -d "$pro_repo/.git" ] && [ ! -f "$pro_repo/.git" ]; then
  echo "ERROR: missing sibling repo: $siblings/bundles" >&2
  echo "This generator reads licensed-plugin counts from a sibling checkout" >&2
  echo "of the private nself-org/bundles repo. Clone it next to this repo." >&2
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
        # A registry entry MAY mirror the manifest's installable flag, and
        # several do. That is fine and not worth reporting. What matters is the
        # two disagreeing, because then a reader of registry.json alone would
        # reach a different count than this generator does. Report only that.
        if isinstance(entry, dict) and "installable" in entry:
            registry_says = bool(entry["installable"])
            if found and registry_says != installable:
                contradictions.append(
                    f"{repo}:{slug} — registry entry says installable="
                    f"{registry_says} but the plugin manifest says "
                    f"{installable}. The manifest wins here, so the count is "
                    f"still correct, but the two must be reconciled: anyone "
                    f"reading registry.json alone will get a different number."
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

# Overlap rule. A slug in both registries is a two-tier product: the free entry
# is its free tier, the pro entry its pro tier (e.g. cron, notify). Both sides
# must carry "tier_pair": true — bundles/scripts/check-bundles-registry-
# consistency.py already enforces that, so an unflagged shared slug is a data
# error rather than a second kind of overlap. One product counts once.
#
# There is deliberately no "count it twice" case. An earlier draft of this
# script invented a "dual_registry" flag for that, but no registry entry has
# ever carried it, so the branch was dead and the real field (tier_pair) went
# unread. Today that still produced 171 because every shared slug happens to be
# a tier pair, but it was luck, not logic.
shared = sorted(set(free_plugins) & set(pro_plugins))
unflagged = sorted(
    s for s in shared
    if not (bool(free_plugins.get(s, {}).get("tier_pair"))
            and bool(pro_plugins.get(s, {}).get("tier_pair")))
)
if unflagged:
    sys.exit(
        "ERROR: slug(s) in both registries without tier_pair:true on both sides: "
        + ", ".join(unflagged)
        + "\nEvery shared slug must be declared a free/pro tier pair. Fix the"
        " registries (see bundles/scripts/check-bundles-registry-consistency.py)"
        " rather than guessing how to count it."
    )
tier_pairs = shared
duplicates = shared

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
        "pro": {"repo": "nself-org/bundles", "sha": blob_sha(pro_repo, "registry.json")},
    },
    "free": free_report,
    "pro": pro_report,
    "overlap": {"sharedSlugs": shared, "tierPairs": tier_pairs, "duplicates": duplicates},
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
        print(f"       tier pairs (free+pro tiers of one product, counted once): "
              f"{', '.join(shared)}")
    print(f"total  {result['totals']['entries']:>4}  installable {result['totals']['installable']:>4}")
    print(f"advertised: {result['advertised']}")

for c in contradictions:
    print(f"DRIFT: {c}", file=sys.stderr)

sys.exit(0)
PYEOF
