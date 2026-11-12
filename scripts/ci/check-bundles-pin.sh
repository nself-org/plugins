#!/usr/bin/env bash
# check-bundles-pin.sh: release gate. Bundle membership in this repo must match nself-org/bundles main at tag time.
#
# Purpose: scripts/bundles.ref pins the bundles commit the catalog is generated from (scripts/plugin-counts.sh). Nothing
#   re-pins when bundles main changes a bundle's membership, so a release could ship a catalog (served by
#   plugins.nself.org) that sells a different plugin set than the licence server grants. This gate runs in the release
#   and fails when the pin, the generated projections, the web pin or the web entitlement rows differ from bundles main.
# Usage:   scripts/ci/check-bundles-pin.sh [--bundles-dir DIR] [--main-ref REF] [--no-fetch]
#                                          [--web-pin FILE] [--entitlements FILE] [--root DIR]
#   --bundles-dir DIR    a clone of nself-org/bundles (default: $BUNDLES_DIR); required, the gate fails closed without it
#   --main-ref REF       the ref that is "bundles main" (default origin/main, or main when the clone has no origin)
#   --no-fetch           do not `git fetch origin main` first (the caller already fetched)
#   --web-pin FILE       a file whose first non-comment line is the bundles sha the web repo pins; it must carry the same
#                        bundles.json as main
#   --entitlements FILE  JSON object {bundle: [plugin, ...]} exported from the web entitlement rows; every bundle in
#                        main's bundles.json must appear with the same plugin set (extra legacy keys are ignored)
#   --root DIR           the plugins checkout to check (default: this script's repo)
# Checks:  1. scripts/bundles.ref is a full sha present in the clone and an ancestor of main.
#          2. bundles.json and registry.json at the pin are byte-identical to main (otherwise the pin is behind).
#          3. catalog.json and counts.json name the pin in generated_from.bundles_ref.
#          4. catalog.json bundle membership equals main's bundles.json membership.
#          5. the optional web pin and entitlement rows (see above).
# Fix:     bump scripts/bundles.ref to bundles main, then run scripts/plugin-counts.sh to regenerate the projections.
# Exit:    0 in sync, 1 drift, 2 usage, 3 precondition missing (no clone, unknown ref, fetch failure, bad pin).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLES_DIR="${BUNDLES_DIR:-}" MAIN_REF="" FETCH=1 WEB_PIN="" ENTITLEMENTS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --bundles-dir) BUNDLES_DIR="${2:-}"; shift 2 ;;
    --main-ref) MAIN_REF="${2:-}"; shift 2 ;;
    --no-fetch) FETCH=0; shift ;;
    --web-pin) WEB_PIN="${2:-}"; shift 2 ;;
    --entitlements) ENTITLEMENTS="${2:-}"; shift 2 ;;
    --root) ROOT="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed '$d;s/^# \{0,1\}//'; exit 0 ;;
    *) echo "check-bundles-pin: unknown argument $1 (see --help)" >&2; exit 2 ;;
  esac
done

pre() { echo "check-bundles-pin: $*" >&2; exit 3; }
command -v jq >/dev/null || pre "jq is required"
[ -n "$BUNDLES_DIR" ] || pre "no bundles clone (pass --bundles-dir or set BUNDLES_DIR); the release cannot verify membership"
git -C "$BUNDLES_DIR" rev-parse --git-dir >/dev/null 2>&1 || pre "$BUNDLES_DIR is not a git clone"

has_origin=0; git -C "$BUNDLES_DIR" remote get-url origin >/dev/null 2>&1 && has_origin=1
if [ -z "$MAIN_REF" ]; then
  if [ "$has_origin" -eq 1 ]; then MAIN_REF=origin/main; else MAIN_REF=main; fi
fi
if [ "$FETCH" -eq 1 ] && [ "$has_origin" -eq 1 ]; then
  git -C "$BUNDLES_DIR" fetch -q origin "+refs/heads/main:refs/remotes/origin/main" \
    || pre "cannot fetch bundles main from origin"
fi
MAIN="$(git -C "$BUNDLES_DIR" rev-parse -q --verify "$MAIN_REF^{commit}" 2>/dev/null)" \
  || pre "bundles main ref $MAIN_REF does not resolve in $BUNDLES_DIR"

read_pin() { grep -v '^#' "$1" | grep -v '^[[:space:]]*$' | head -1 | tr -d '[:space:]'; }
[ -f "$ROOT/scripts/bundles.ref" ] || pre "$ROOT/scripts/bundles.ref is missing"
PIN="$(read_pin "$ROOT/scripts/bundles.ref")"
[[ "$PIN" =~ ^[0-9a-f]{40}$ ]] || pre "scripts/bundles.ref must hold a full commit sha (got '$PIN')"
if ! git -C "$BUNDLES_DIR" cat-file -e "$PIN^{commit}" 2>/dev/null && [ "$has_origin" -eq 1 ] && [ "$FETCH" -eq 1 ]; then
  git -C "$BUNDLES_DIR" fetch -q origin "$PIN" 2>/dev/null || true
fi
git -C "$BUNDLES_DIR" cat-file -e "$PIN^{commit}" 2>/dev/null || pre "$BUNDLES_DIR does not hold $PIN (scripts/bundles.ref)"

drift=0
bad() { echo "check-bundles-pin: DRIFT: $*" >&2; drift=1; }
blob() { git -C "$BUNDLES_DIR" rev-parse -q --verify "$1:$2" 2>/dev/null || echo "missing"; }

# 1-2. The pin is on main and carries main's licensed data.
git -C "$BUNDLES_DIR" merge-base --is-ancestor "$PIN" "$MAIN" \
  || bad "scripts/bundles.ref $PIN is not an ancestor of bundles $MAIN_REF ($MAIN)"
for f in bundles.json registry.json; do
  [ "$(blob "$MAIN" "$f")" != missing ] || pre "bundles $MAIN_REF has no $f"
  [ "$(blob "$PIN" "$f")" = "$(blob "$MAIN" "$f")" ] \
    || bad "scripts/bundles.ref $PIN is behind bundles $MAIN_REF ($MAIN): $f differs; bump the pin and run scripts/plugin-counts.sh"
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/check-bundles-pin.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
git -C "$BUNDLES_DIR" show "$MAIN:bundles.json" > "$WORK/main-bundles.json"
members() { jq -S '.bundles | map_values(.plugins | sort)' "$1"; }
members "$WORK/main-bundles.json" > "$WORK/main-members.json" || pre "bundles $MAIN_REF bundles.json is not valid JSON"

# 3. The projections were generated from the pin.
for f in catalog.json counts.json; do
  [ -f "$ROOT/$f" ] || { bad "$f is missing"; continue; }
  got="$(jq -r '.generated_from.bundles_ref // ""' "$ROOT/$f" 2>/dev/null || echo "")"
  [ "$got" = "$PIN" ] || bad "$f was generated from bundles_ref '$got', not the pin $PIN; run scripts/plugin-counts.sh"
done

# 4. catalog.json membership equals main.
if [ -f "$ROOT/catalog.json" ]; then
  members "$ROOT/catalog.json" > "$WORK/catalog-members.json" 2>/dev/null || echo '{}' > "$WORK/catalog-members.json"
  if ! diff -u "$WORK/main-members.json" "$WORK/catalog-members.json" > "$WORK/members.diff"; then
    bad "catalog.json bundle membership differs from bundles $MAIN_REF bundles.json:"; sed 's/^/  /' "$WORK/members.diff" >&2
  fi
fi

# 5. The web side.
if [ -n "$WEB_PIN" ]; then
  [ -f "$WEB_PIN" ] || pre "web pin file $WEB_PIN is missing"
  wpin="$(read_pin "$WEB_PIN")"
  if ! [[ "$wpin" =~ ^[0-9a-f]{40}$ ]] || ! git -C "$BUNDLES_DIR" cat-file -e "$wpin^{commit}" 2>/dev/null; then
    bad "web pin $WEB_PIN holds '$wpin', not a bundles commit"
  elif [ "$(blob "$wpin" bundles.json)" != "$(blob "$MAIN" bundles.json)" ]; then
    bad "web pin $wpin is behind bundles $MAIN_REF ($MAIN): bundles.json differs"
  fi
fi
if [ -n "$ENTITLEMENTS" ]; then
  [ -f "$ENTITLEMENTS" ] || pre "entitlements file $ENTITLEMENTS is missing"
  jq -e 'type == "object"' "$ENTITLEMENTS" >/dev/null 2>&1 || pre "entitlements file $ENTITLEMENTS must be a JSON object"
  missing="$(jq -r --slurpfile e "$ENTITLEMENTS" 'to_entries[] | select(($e[0][.key] // null | if . == null then null else sort end) != .value) | .key' \
    "$WORK/main-members.json")"
  for b in $missing; do bad "web entitlement row for bundle '$b' differs from bundles $MAIN_REF bundles.json"; done
fi

if [ "$drift" -ne 0 ]; then echo "check-bundles-pin: FAILED (bundles $MAIN_REF is $MAIN, pin is $PIN)" >&2; exit 1; fi
echo "check-bundles-pin: OK (pin $PIN matches bundles $MAIN_REF $MAIN)"
