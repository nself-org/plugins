#!/usr/bin/env bash
# verify-published-checksums.sh
#
# Purpose: spot-check that the checksum registry.json WOULD carry for a
# release tag matches the SHA-256 of the tarball actually served for that
# tag on the GitHub release. This never writes registry.json — it downloads
# each plugin's real tarball asset for TAG and diffs the computed SHA-256
# against the registry's checksum/checksums.sha256 (if either is already
# populated for that tag) and prints the computed value regardless, so it
# can seed a future registry update by hand or via build-and-upload-tarballs.sh.
#
# Why this exists (P6, registry-flat-checksum-field fix): most of this
# registry's 129 plugins have never had ANY checksum field computed against
# their real published v1.0.0 asset — recomputing all of them from the repo
# source tree would NOT verify what a user's `nself plugin install` actually
# downloads, since the published tarball's bytes depend on the exact `tar`
# invocation/timestamps used at release time, not just file contents. This
# script downloads the real released asset and hashes THAT.
#
# Inputs: TAG (e.g. v1.0.0), PLUGIN_NAMES (space-separated plugin slugs).
# Outputs: a table of plugin / computed sha256 / registry checksum /
#          registry checksums.sha256 / match — to stdout. Exit 0 always
#          (informational spot-check, not a CI gate); pass --strict to
#          exit 1 on any mismatch against a NON-EMPTY registry value.
# Constraints: read-only against registry.json and GitHub; requires gh CLI
#              authenticated + jq + sha256sum/shasum. Never edits registry.json.
#
# Usage:
#   ./scripts/verify-published-checksums.sh v1.0.0 storage cron notify search maintenance
#   ./scripts/verify-published-checksums.sh --strict v1.0.0 ollama

set -euo pipefail

REPO="nself-org/plugins"
STRICT=false

if [ "${1:-}" = "--strict" ]; then
  STRICT=true
  shift
fi

TAG="${1:-}"
shift || true
PLUGIN_NAMES=("$@")

if [ -z "$TAG" ] || [ "${#PLUGIN_NAMES[@]}" -eq 0 ]; then
  printf "Usage: %s [--strict] TAG PLUGIN_NAME [PLUGIN_NAME ...]\n" "$0" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY_FILE="$(cd "${SCRIPT_DIR}/.." && pwd)/registry.json"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

log() { printf "[verify-published-checksums] %s\n" "$*"; }
err() { printf "[verify-published-checksums] ERROR: %s\n" "$*" >&2; }

if ! command -v gh >/dev/null 2>&1; then
  err "gh CLI not found."
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  err "jq not found."
  exit 1
fi

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

if ! gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  err "Release $TAG not found in $REPO."
  exit 1
fi

MISMATCHES=0

printf "%-20s %-10s %-64s %-10s %-10s\n" "plugin" "version" "computed_sha256" "reg.flat" "reg.nested"
printf "%-20s %-10s %-64s %-10s %-10s\n" "------" "-------" "---------------" "--------" "----------"

for plugin_name in "${PLUGIN_NAMES[@]}"; do
  version="${TAG#v}"
  tarball_name="${plugin_name}-${version}.tar.gz"
  asset_path="${WORK_DIR}/${tarball_name}"

  if ! gh release download "$TAG" --repo "$REPO" --pattern "$tarball_name" --dir "$WORK_DIR" --clobber >/dev/null 2>&1; then
    printf "%-20s %-10s %-64s %-10s %-10s\n" "$plugin_name" "$version" "(asset not found on $TAG)" "-" "-"
    continue
  fi

  computed_sha="$(sha256_file "$asset_path")"

  reg_flat="$(jq -r --arg n "$plugin_name" '.plugins[$n].checksum // ""' "$REGISTRY_FILE")"
  reg_nested="$(jq -r --arg n "$plugin_name" '.plugins[$n].checksums.sha256 // ""' "$REGISTRY_FILE")"
  reg_nested_norm="${reg_nested#sha256:}"

  match_flag=""
  if [ -n "$reg_flat" ] && [ "$reg_flat" != "$computed_sha" ]; then
    match_flag="${match_flag} FLAT-MISMATCH"
    MISMATCHES=$((MISMATCHES + 1))
  fi
  if [ -n "$reg_nested_norm" ] && [ "$reg_nested_norm" != "$computed_sha" ]; then
    match_flag="${match_flag} NESTED-MISMATCH"
    MISMATCHES=$((MISMATCHES + 1))
  fi
  [ -z "$match_flag" ] && match_flag="ok-or-unset"

  printf "%-20s %-10s %-64s %-10s %-10s\n" \
    "$plugin_name" "$version" "$computed_sha" "${reg_flat:--}" "${reg_nested:--}"
  log "  ${plugin_name}: ${match_flag}"
done

if [ "$MISMATCHES" -gt 0 ]; then
  err "$MISMATCHES mismatch(es) between a computed checksum and a NON-EMPTY registry value."
  if [ "$STRICT" = "true" ]; then
    exit 1
  fi
else
  log "No mismatches against any non-empty registry checksum field."
fi

log "Done. Registry.json was NOT modified by this script."
