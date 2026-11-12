#!/usr/bin/env bash
# licensed-registry-contract.sh
#
# Purpose: the registry-row equals ping-bytes contract (P7-WIRE-16, D-1483). For every licensed slug
#   (tier pro or max, or requires_license) in bundles registry.json at REF, the row's checksum must equal
#   the sha256 of the bytes ping serves for that slug. The CLI checks exactly this pair at install time;
#   a mismatch is `nself plugin install <slug>` failing E124.
# Inputs: --ref REF (default: LICENSED_REF in [env.production.vars] of the registry worker's wrangler.toml);
#   --registry FILE (registry.json for REF; default: fetched with `gh api` from nself-org/bundles at REF);
#   --plugin SLUG (check one slug). Env: NSELF_PLUGIN_LICENSE_KEY_OWNER (license key sent as X-License-Key),
#   PING_API_BASE (default https://ping.nself.org; any curl URL, file:// included).
# Outputs: one "ok"/"MISMATCH"/"FAIL" line per licensed slug, then a summary.
# Exit: 0 every licensed row matches; 1 a mismatch, a failed download, a licensed row without a checksum, or no
#   licensed rows at all; 2 usage; 3 precondition unmet (no jq/curl, no ref, no license key, registry unreadable).
# Constraints: bash 3.2 compatible; read-only (never writes registry.json); ping cannot report which ref it
#   builds from, so REF is the operator's assertion (ping's PLUGINS_PRO_REF) and this proves the pair at it.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
wrangler="$repo/.workers/plugins-registry/wrangler.toml"
PING_API_BASE="${PING_API_BASE:-https://ping.nself.org}"

ref="" registry="" only=""
usage() { sed -n '3,17p' "$0" | sed 's/^# \{0,1\}//'; }
while [ $# -gt 0 ]; do
  case "$1" in
    --ref) ref="${2:-}"; shift 2 || { usage >&2; exit 2; } ;;
    --registry) registry="${2:-}"; shift 2 || { usage >&2; exit 2; } ;;
    --plugin) only="${2:-}"; shift 2 || { usage >&2; exit 2; } ;;
    -h|--help) usage; exit 0 ;;
    *) echo "licensed-registry-contract: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

for tool in jq curl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "licensed-registry-contract: $tool is required" >&2; exit 3; }
done

# The production ref the worker is pinned to, read from the one place it is set.
prod_ref() {
  awk '
    /^\[/ { in_prod = ($0 == "[env.production.vars]") ; next }
    in_prod && /^LICENSED_REF[ \t]*=/ { sub(/^[^=]*=[ \t]*"/, ""); sub(/".*$/, ""); print; exit }
  ' "$wrangler" 2>/dev/null
}
[ -n "$ref" ] || ref="$(prod_ref)"
if [ -z "$ref" ]; then
  echo "licensed-registry-contract: no ref (pass --ref, or set LICENSED_REF in [env.production.vars] of $wrangler)" >&2
  exit 3
fi

key="${NSELF_PLUGIN_LICENSE_KEY_OWNER:-}"
if [ -z "$key" ]; then
  echo "licensed-registry-contract: NSELF_PLUGIN_LICENSE_KEY_OWNER is unset; licensed bytes cannot be fetched" >&2
  exit 3
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/licensed-contract.XXXXXX")" || exit 3
trap 'rm -rf "$work"' EXIT

if [ -z "$registry" ]; then
  command -v gh >/dev/null 2>&1 || { echo "licensed-registry-contract: gh is required without --registry" >&2; exit 3; }
  registry="$work/registry.json"
  if ! gh api "repos/nself-org/bundles/contents/registry.json?ref=$ref" --jq .content 2>/dev/null \
      | tr -d '\n' | base64 -d > "$registry" 2>/dev/null || ! jq -e . "$registry" >/dev/null 2>&1; then
    echo "licensed-registry-contract: cannot read nself-org/bundles registry.json at $ref" >&2
    exit 3
  fi
fi
jq -e '.plugins' "$registry" >/dev/null 2>&1 || { echo "licensed-registry-contract: no .plugins in $registry" >&2; exit 3; }

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# slug<TAB>checksum for each licensed row; the plugins map and the array form both appear in the wild.
jq -r --arg only "$only" '
  .plugins
  | (if type == "object" then to_entries | map(.value + {name: (.value.name // .key)}) else . end)[]
  | select((.tier == "pro") or (.tier == "max") or (.requires_license == true))
  | select($only == "" or .name == $only)
  | [.name, (.checksum // "")] | @tsv
' "$registry" > "$work/rows.tsv" || exit 3

matched=0 failed=0
while IFS="$(printf '\t')" read -r slug want; do
  [ -n "$slug" ] || continue
  case "$slug" in *[!A-Za-z0-9._-]*) echo "FAIL      $slug: not a plugin slug"; failed=$((failed + 1)); continue ;; esac
  want="$(printf '%s' "${want#sha256:}" | tr 'A-F' 'a-f')"
  if [ -z "$want" ]; then
    echo "FAIL      $slug: licensed row at $ref has no checksum"; failed=$((failed + 1)); continue
  fi
  out="$work/$slug.tar.gz"
  if ! curl -sSLf --max-time 120 -H "X-License-Key: $key" -o "$out" "$PING_API_BASE/plugins/$slug/download" 2>/dev/null; then
    echo "FAIL      $slug: download failed from $PING_API_BASE"; failed=$((failed + 1)); continue
  fi
  got="$(sha256_of "$out")"
  if [ "$got" = "$want" ]; then
    echo "ok        $slug"; matched=$((matched + 1))
  else
    echo "MISMATCH  $slug: row at $ref says $want, ping serves $got"; failed=$((failed + 1))
  fi
done < "$work/rows.tsv"

echo "licensed-registry-contract: ref $ref: $matched match, $failed fail"
if [ $((matched + failed)) -eq 0 ]; then
  echo "licensed-registry-contract: no licensed rows${only:+ named $only} at $ref" >&2
  exit 1
fi
[ "$failed" -eq 0 ] || exit 1
exit 0
