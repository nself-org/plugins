#!/usr/bin/env bash
# plugin-counts.sh: the one entrypoint that writes registry.json, catalog.json and counts.json.
#
# Purpose: the three files are projections (ADR 0009, Epic P7-PLUG D3). The pinned cli tools/catalog generates them from
#   free/*/plugin.json, releases.json and the licensed side (registry.json + bundles.json of nself-org/bundles at
#   scripts/bundles.ref). Nobody edits them; this script is the one regenerate command and the drift gate.
# Usage:   scripts/plugin-counts.sh [--check [--free-only]] | --json | --init-releases
#   (none)            rewrite registry.json, catalog.json and counts.json
#   --json            write nothing; print the generated counts.json on stdout (the old read-only use)
#   --check           write nothing; exit 1 and print the diff when a projection is not what the generator produces
#   --free-only       with --check, for CI without the bundles token: verify registry.json only, taking the bundle
#                     membership and the licensed slugs from the committed catalog.json (catalog.json and counts.json
#                     are NOT verified; that needs the bundles checkout)
#   --init-releases   one-time: write releases.json from the hand-kept registry.json now in the tree
#                     (scripts/catalog/releases-from-registry.jq); refuses when releases.json exists or registry.json is
#                     already generated (releases.json is written only from published bytes, by verify-published-checksums.sh)
# Inputs:  scripts/cli-tools.version (cli commit holding tools/catalog), scripts/bundles.ref, free/*/plugin.json,
#          releases.json, scripts/catalog/licensed-catalog-carry.json.
#   Env:   BUNDLES_DIR    a clone of nself-org/bundles (default ../bundles, ../plugins-pro, under $NSELF, or beside this repo from a worktree); only the pinned
#                         commit is read, so the clone's branch and working tree do not matter
#          CATALOG_BIN    use this generator binary instead of building one (offline and test runs)
#          CLI_TOOLS_SRC  build from this cli checkout instead of fetching the pinned commit
#          TODAY          YYYY-MM-DD override for the carry expiry check
# Wire:    released CLIs (v1.4.x installLocked) install EVERY listed dependency edge, optional ones included, and fail the
#          install on a name they cannot fetch or a licensed plugin without a licence. So registry.json lists only the
#          required edges whose target is a free registry entry; every other edge (optional, licensed-tier, unreleased or a
#          core service such as `database`) stays in the manifest and is left off the wire, named on stderr.
# Exit:    0 ok, 1 drift or carry expired, 2 usage or generator failure, 3 precondition missing.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CARRY="$ROOT/scripts/catalog/licensed-catalog-carry.json"
CHECK=0 FREE_ONLY=0 INIT=0 JSON=0
for arg in "$@"; do
  case "$arg" in
    --check) CHECK=1 ;;
    --free-only) FREE_ONLY=1 ;;
    --init-releases) INIT=1 ;;
    --json) JSON=1 ;;
    -h|--help) sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed '$d;s/^# \{0,1\}//'; exit 0 ;;
    *) echo "plugin-counts: unknown argument $arg (see --help)" >&2; exit 2 ;;
  esac
done
[ "$FREE_ONLY" -eq 0 ] || [ "$CHECK" -eq 1 ] || { echo "plugin-counts: --free-only needs --check" >&2; exit 2; }
[ "$JSON" -eq 0 ] || { [ "$CHECK" -eq 0 ] && [ "$INIT" -eq 0 ]; } || { echo "plugin-counts: --json cannot be combined with --check or --init-releases" >&2; exit 2; }
[ "$INIT" -eq 0 ] || [ "$CHECK" -eq 0 ] || { echo "plugin-counts: --init-releases cannot be combined with --check" >&2; exit 2; }

for tool in jq git diff; do
  command -v "$tool" >/dev/null || { echo "plugin-counts: $tool is required" >&2; exit 3; }
done

if [ "$INIT" -eq 1 ]; then
  if [ -e "$ROOT/releases.json" ] || [ "$(jq -r 'has("_generated")' "$ROOT/registry.json")" = "true" ]; then
    echo "plugin-counts: --init-releases is a one-time extraction from a hand-kept registry.json; releases.json is written by verify-published-checksums.sh" >&2; exit 1
  fi
  jq -S --indent 2 -f "$ROOT/scripts/catalog/releases-from-registry.jq" "$ROOT/registry.json" > "$ROOT/releases.json.tmp"
  mv "$ROOT/releases.json.tmp" "$ROOT/releases.json"
  echo "plugin-counts: wrote releases.json ($(jq '.releases | length' "$ROOT/releases.json") rows) from registry.json"
  exit 0
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/plugin-counts.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
BUNDLES_REF="$(grep -v '^#' "$ROOT/scripts/bundles.ref" | head -1 | tr -d '[:space:]')"
[[ "$BUNDLES_REF" =~ ^[0-9a-f]{40}$ ]] || { echo "plugin-counts: scripts/bundles.ref must hold a full commit sha" >&2; exit 3; }
[ -f "$ROOT/releases.json" ] || { echo "plugin-counts: releases.json is missing (run --init-releases once)" >&2; exit 3; }

# Licensed side. Full mode reads registry.json and bundles.json from the pinned bundles commit; --free-only rebuilds the
# two things the free registry needs (bundle membership, licensed slugs) from the committed catalog.json.
if [ "$FREE_ONLY" -eq 1 ]; then
  jq '{schema_version: "2.0.0", bundles: (.bundles | map_values({display, plugins, tier: (if .tier == "free" then "free" else "paid" end)}))}' \
    "$ROOT/catalog.json" > "$WORK/bundles.json"
  jq '{plugins: ([.plugins[] | select(.tier == "licensed") | {key: .slug, value: {}}] | from_entries)}' \
    "$ROOT/catalog.json" > "$WORK/licensed.json"
else
  BUNDLES_DIR="${BUNDLES_DIR:-}"
  COMMON="$(git -C "$ROOT" rev-parse --git-common-dir 2>/dev/null || echo /nonexistent)"; [[ "$COMMON" = /* ]] || COMMON="$ROOT/$COMMON"
  if [ -z "$BUNDLES_DIR" ]; then
    for d in "$ROOT/../bundles" "$ROOT/../plugins-pro" "${NSELF:-/nonexistent}/plugins-pro" "${NSELF:-/nonexistent}/bundles" "$COMMON/../../plugins-pro"; do [ -d "$d/.git" ] || [ -f "$d/.git" ] && { BUNDLES_DIR="$d"; break; }; done
  fi
  [ -n "$BUNDLES_DIR" ] || { echo "plugin-counts: no bundles checkout (set BUNDLES_DIR, or use --check --free-only)" >&2; exit 3; }
  if ! git -C "$BUNDLES_DIR" cat-file -e "$BUNDLES_REF^{commit}" 2>/dev/null; then
    git -C "$BUNDLES_DIR" fetch -q origin "$BUNDLES_REF" 2>/dev/null || true
  fi
  git -C "$BUNDLES_DIR" cat-file -e "$BUNDLES_REF^{commit}" 2>/dev/null \
    || { echo "plugin-counts: $BUNDLES_DIR does not hold $BUNDLES_REF (scripts/bundles.ref)" >&2; exit 3; }
  git -C "$BUNDLES_DIR" show "$BUNDLES_REF:bundles.json" > "$WORK/bundles.json"
  git -C "$BUNDLES_DIR" show "$BUNDLES_REF:registry.json" > "$WORK/licensed.json"
fi

# The generator: the pinned cli commit built offline from its vendor tree (go run @ref cannot work, cli go.mod has replace
# directives). conformance/lib.sh owns the fetch, so this gate and the conformance gate share one cached checkout.
CATALOG="${CATALOG_BIN:-}"
if [ -z "$CATALOG" ]; then
  command -v go >/dev/null || { echo "plugin-counts: go is required to build the generator" >&2; exit 3; }
  # shellcheck source=conformance/lib.sh
  . "$ROOT/scripts/conformance/lib.sh"
  CATALOG="$(cli_tool catalog)" || { echo "plugin-counts: cannot build tools/catalog at the pinned cli commit" >&2; exit 3; }
fi

run_generator() {
  "$CATALOG" "$@" > /dev/null 2> "$WORK/catalog.log" || { cat "$WORK/catalog.log" >&2; exit 2; }
  cat "$WORK/catalog.log" >&2
}
run_generator -mode free -plugins "$ROOT/free" -releases "$ROOT/releases.json" -bundles "$WORK/bundles.json" \
  -peer-registry "$WORK/licensed.json" -bundles-ref "$BUNDLES_REF" -out "$WORK/out"
# Wire correction (see Wire above): keep required edges to free registry entries, name every other edge.
jq -r '(.plugins | keys) as $free | .plugins | to_entries[] | .key as $k | .value.dependencies
  | (if type == "object" then (.required | map({d: ., why: "required"})) + (.optional | map({d: ., why: "optional"})) else map({d: ., why: "required"}) end)[]
  | select((.why == "optional") or (.d as $d | $free | index($d) | not))
  | "catalog: note: \($k) \(.why) dependency \(.d) is not a required free registry entry; left off the wire"' \
  "$WORK/out/registry.json" >&2
jq -S --indent 2 '(.plugins | keys) as $free
  | .plugins |= map_values(.dependencies |= ((if type == "object" then .required else . end) | map(select(. as $d | $free | index($d)))))' \
  "$WORK/out/registry.json" > "$WORK/registry.wire.json"
mv "$WORK/registry.wire.json" "$WORK/out/registry.json"
FILES=(registry.json)

if [ "$FREE_ONLY" -eq 0 ]; then
  # The carried licensed entries (bundles scripts/registry-carry.json) have no catalog block: add the one from the carry
  # file, to entries that lack one, until it expires.
  TODAY="${TODAY:-$(date -u +%Y-%m-%d)}"
  expires="$(jq -er '.expires' "$CARRY")" || { echo "plugin-counts: $CARRY needs an expires date" >&2; exit 1; }
  [[ "$TODAY" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "plugin-counts: TODAY must be YYYY-MM-DD" >&2; exit 2; }
  [[ "$expires" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "plugin-counts: $CARRY needs an expires date (YYYY-MM-DD)" >&2; exit 1; }
  if [[ "$TODAY" > "$expires" ]]; then
    echo "plugin-counts: the carry in scripts/catalog/licensed-catalog-carry.json expired on $expires; fix the manifests it names" >&2; exit 1
  fi
  jq --slurpfile c "$CARRY" '.plugins |= with_entries(if .value.catalog == null then .value.catalog = ($c[0].entries[.key].catalog // error("licensed entry " + .key + " has no catalog block and no carry")) else . end)' \
    "$WORK/licensed.json" > "$WORK/licensed-cat.json" || { echo "plugin-counts: a licensed entry has no catalog block and no carry entry" >&2; exit 1; }
  run_generator -mode catalog -free-registry "$WORK/out/registry.json" -licensed-registry "$WORK/licensed-cat.json" \
    -bundles "$WORK/bundles.json" -bundles-ref "$BUNDLES_REF" -out "$WORK/out"
  FILES+=(catalog.json counts.json)
fi

if [ "$JSON" -eq 1 ]; then cat "$WORK/out/counts.json"; exit 0; fi
if [ "$CHECK" -eq 1 ]; then
  stale=0
  for f in "${FILES[@]}"; do
    if ! diff -u "$ROOT/$f" "$WORK/out/$f" > "$WORK/$f.diff"; then
      head -n 60 "$WORK/$f.diff" >&2
      echo "plugin-counts: $f is stale; run scripts/plugin-counts.sh and commit the result" >&2
      stale=1
    fi
  done
  [ "$stale" -eq 0 ] || exit 1
  echo "plugin-counts: ${FILES[*]} current"
  exit 0
fi
for f in "${FILES[@]}"; do cp "$WORK/out/$f" "$ROOT/$f"; done
echo "plugin-counts: wrote ${FILES[*]} ($(jq '.plugins_count' "$ROOT/registry.json") free plugins, advertised $(jq '.advertised' "$ROOT/counts.json"))"
