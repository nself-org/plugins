#!/usr/bin/env bash
# verify-published-checksums.sh
#
# Purpose: the release model this repo ships under (see .github/RELEASE-
# v1.2.1.md and tarball-checksum-gate.yml) treats the PUBLISHED release
# asset as the artifact of record — registry.json's checksums are supposed
# to be written FROM those published bytes, not from an independent local
# rebuild that merely hopes to match (gzip's DEFLATE output is
# implementation/version-dependent; two byte-identical tar streams can
# still produce different .tar.gz bytes on different machines — see
# tarball-checksum-gate.yml's header comment for the measured proof). This
# script is the one place that reads real published bytes and either
# reports how registry.json disagrees with them, or writes them in.
#
# Inputs: TAG (e.g. v1.2.1) — a release that must already exist (the owner
# creates it as a draft first; `gh release download` works against a draft
# with repo access). registry.json in the current directory is read (and,
# with --write, rewritten).
#
# For every registry.json entry whose releaseTag equals TAG and which
# carries a checksum-eligible tarball (i.e. NOT shared-utils-style
# installable:false entries with no tarball/checksum fields at all), this
# downloads:
#   - the source tarball  <name>-<version>.tar.gz
#   - if the plugin's free/<name>/plugin.json declares a binaryName (or the
#     registry entry already has checksums.platforms), the five per-platform
#     binary tarballs <name>-<version>-<platform>.tar.gz
# and computes sha256 for each.
#
# Outputs:
#   Default (report mode): prints ONLY the mismatches found (registry value
#     vs. the freshly computed published-asset hash) and a final summary
#     line; registry.json is NOT modified. Exit 0 if no mismatches, exit 1
#     if any (whether a value disagreed or an expected asset was simply
#     missing from the release).
#   --write: writes the computed hashes into registry.json — the flat
#     `checksum` field, the nested `checksums.sha256` field (both from the
#     source tarball), and `checksums.platforms.<platform>` for each
#     downloaded platform asset — then reports what changed. Exit 0 on
#     success, exit 1 if any expected asset could not be downloaded (in
#     which case registry.json is left with whatever it already had for
#     that specific field; nothing partial is written for a missing asset).
#
# Constraints: requires `gh` (authenticated, repo access — including to a
# draft release), `jq`, and `sha256sum`/`shasum`. Never touches anything
# but registry.json. Never creates or publishes a release, never tags.
#
# Usage:
#   ./scripts/verify-published-checksums.sh v1.2.1
#   ./scripts/verify-published-checksums.sh --write v1.2.1

set -euo pipefail

REPO="nself-org/plugins"
WRITE=false

if [ "${1:-}" = "--write" ]; then
  WRITE=true
  shift
fi

TAG="${1:-}"

if [ -z "$TAG" ]; then
  printf "Usage: %s [--write] TAG\n" "$0" >&2
  printf "  e.g. %s v1.2.1\n" "$0" >&2
  printf "       %s --write v1.2.1\n" "$0" >&2
  exit 1
fi

VERSION="${TAG#v}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REGISTRY_FILE="${REPO_ROOT}/registry.json"
PLUGINS_DIR="${REPO_ROOT}/free"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

# Matches internal/plugin/arch.go's PlatformArch() — the same list
# scripts/build-tarballs.sh cross-compiles for a binaryName plugin.
PLATFORMS="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64"

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

if [ ! -f "$REGISTRY_FILE" ]; then
  err "registry.json not found at ${REGISTRY_FILE}"
  exit 1
fi

if ! gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  err "Release $TAG not found in $REPO (draft releases work fine with repo access — create it first)."
  exit 1
fi

REGISTRY_JSON="$(cat "$REGISTRY_FILE")"
MISMATCHES=0
MISSING_ASSETS=0
DOWNLOAD_ERRORS=0
CHECKED=0
WRITTEN=0

# Fetch the release's asset list ONCE, and fetch the bytes ONCE.
#
# This used to run `gh release download --pattern <name>` per asset: 578 API
# calls for v1.2.1. That is slow, it trips GitHub's secondary rate limit (the
# same limit that broke the upload job at 525 assets), and — because the call
# was `>/dev/null 2>&1` — a throttled or transient download was indistinguishable
# from a genuinely absent asset and got reported as "MISSING". On the first
# v1.2.1 run that produced a false MISSING for flags/linux-arm64, an asset which
# was in fact present on the release. A checksum gate that cries missing under
# load is worse than no gate: the failure mode is to dismiss it as noise.
#
# So: list once, download everything once, then work from local files. An asset
# absent from the manifest is MISSING (a real release defect). An asset that is
# in the manifest but has no local file is a DOWNLOAD ERROR (transient/infra).
# They are counted separately and mean different things.
ASSET_MANIFEST="${WORK_DIR}/.asset-manifest"
if ! gh release view "$TAG" --repo "$REPO" --json assets --jq '.assets[].name' > "$ASSET_MANIFEST" 2>/dev/null; then
  err "could not list assets on release $TAG"
  exit 1
fi
log "Release $TAG lists $(wc -l < "$ASSET_MANIFEST" | tr -d ' ') asset(s); downloading them once."

asset_is_published() { grep -Fxq "$1" "$ASSET_MANIFEST"; }

dl_err="${WORK_DIR}/.download-stderr"
if ! gh release download "$TAG" --repo "$REPO" --dir "$WORK_DIR" --clobber >/dev/null 2>"$dl_err"; then
  err "bulk download of $TAG's assets failed; cannot verify against published bytes."
  err "gh said: $(tr '\n' ' ' < "$dl_err" | cut -c1-400)"
  exit 1
fi
rm -f "$dl_err"

# Iterate plugin names in a stable, deterministic order rather than
# registry.json's own (already-alphabetical) key order directly, so this
# script's log output is predictable even if that ever changes.
while IFS= read -r name; do
  entry="$(printf '%s' "$REGISTRY_JSON" | jq -c --arg n "$name" '.plugins[$n]')"

  entry_tag="$(printf '%s' "$entry" | jq -r '.releaseTag // empty')"
  if [ "$entry_tag" != "$TAG" ]; then
    # Not on this release — nothing published under TAG to check it against.
    continue
  fi

  has_checksum_field="$(printf '%s' "$entry" | jq -r 'has("checksum") or (has("checksums") and (.checksums | has("sha256")))')"
  if [ "$has_checksum_field" != "true" ]; then
    # e.g. shared-utils: installable:false, no tarball, nothing to verify.
    continue
  fi

  tarball_name="${name}-${VERSION}.tar.gz"
  asset_path="${WORK_DIR}/${tarball_name}"

  if ! asset_is_published "$tarball_name"; then
    printf "MISSING %s: no asset named %s on release %s\n" "$name" "$tarball_name" "$TAG"
    MISSING_ASSETS=$((MISSING_ASSETS + 1))
    continue
  fi
  if [ ! -f "$asset_path" ]; then
    printf "DOWNLOAD-ERROR %s: %s is published but was not downloaded\n" "$name" "$tarball_name"
    DOWNLOAD_ERRORS=$((DOWNLOAD_ERRORS + 1))
    continue
  fi

  computed_sha="$(sha256_file "$asset_path")"
  CHECKED=$((CHECKED + 1))

  reg_flat="$(printf '%s' "$entry" | jq -r '.checksum // ""')"
  reg_nested="$(printf '%s' "$entry" | jq -r '.checksums.sha256 // ""' )"
  reg_nested_hex="${reg_nested#sha256:}"

  if [ "$WRITE" = "true" ]; then
    REGISTRY_JSON="$(printf '%s' "$REGISTRY_JSON" | jq \
      --arg name "$name" --arg sha "$computed_sha" \
      '(.plugins[$name].checksum) = $sha | (.plugins[$name].checksums.sha256) = $sha')"
    WRITTEN=$((WRITTEN + 1))
  else
    if [ -n "$reg_flat" ] && [ "$reg_flat" != "$computed_sha" ]; then
      printf "MISMATCH %s: .checksum=%s  published=%s\n" "$name" "$reg_flat" "$computed_sha"
      MISMATCHES=$((MISMATCHES + 1))
    fi
    if [ -n "$reg_nested_hex" ] && [ "$reg_nested_hex" != "$computed_sha" ]; then
      printf "MISMATCH %s: .checksums.sha256=%s  published=%s\n" "$name" "$reg_nested" "$computed_sha"
      MISMATCHES=$((MISMATCHES + 1))
    fi
  fi

  # Per-platform assets: only for plugins that actually ship a binary —
  # either the registry entry already carries checksums.platforms, or the
  # plugin's own manifest declares a binaryName (covers the "not yet
  # populated" case, e.g. a brand-new binaryName plugin with nothing under
  # checksums.platforms yet — --write should still be able to seed it).
  plugin_json="${PLUGINS_DIR}/${name}/plugin.json"
  bin_name=""
  if [ -f "$plugin_json" ]; then
    bin_name="$(jq -r '.binaryName // .implementation.binaryName // ""' "$plugin_json" 2>/dev/null || printf '')"
  fi
  has_platforms_field="$(printf '%s' "$entry" | jq -r '(.checksums.platforms // {}) | length > 0')"

  if [ -n "$bin_name" ] || [ "$has_platforms_field" = "true" ]; then
    for platform in $PLATFORMS; do
      ptar_name="${name}-${VERSION}-${platform}.tar.gz"
      ptar_path="${WORK_DIR}/${ptar_name}"

      if ! asset_is_published "$ptar_name"; then
        printf "MISSING %s/%s: no asset named %s on release %s\n" "$name" "$platform" "$ptar_name" "$TAG"
        MISSING_ASSETS=$((MISSING_ASSETS + 1))
        continue
      fi
      if [ ! -f "$ptar_path" ]; then
        printf "DOWNLOAD-ERROR %s/%s: %s is published but was not downloaded\n" "$name" "$platform" "$ptar_name"
        DOWNLOAD_ERRORS=$((DOWNLOAD_ERRORS + 1))
        continue
      fi

      p_computed="$(sha256_file "$ptar_path")"
      CHECKED=$((CHECKED + 1))

      if [ "$WRITE" = "true" ]; then
        REGISTRY_JSON="$(printf '%s' "$REGISTRY_JSON" | jq \
          --arg name "$name" --arg p "$platform" --arg sha "$p_computed" \
          '(.plugins[$name].checksums.platforms[$p]) = $sha')"
        WRITTEN=$((WRITTEN + 1))
      else
        reg_platform="$(printf '%s' "$entry" | jq -r --arg p "$platform" '.checksums.platforms[$p] // ""')"
        if [ -n "$reg_platform" ] && [ "$reg_platform" != "$p_computed" ]; then
          printf "MISMATCH %s/%s: .checksums.platforms.%s=%s  published=%s\n" \
            "$name" "$platform" "$platform" "$reg_platform" "$p_computed"
          MISMATCHES=$((MISMATCHES + 1))
        fi
      fi
    done
  fi
done < <(printf '%s' "$REGISTRY_JSON" | jq -r '.plugins | keys[]')

if [ "$WRITE" = "true" ]; then
  printf '%s\n' "$REGISTRY_JSON" > "$REGISTRY_FILE"
  log "Wrote ${WRITTEN} checksum field(s) into registry.json from ${TAG}'s published assets (checked ${CHECKED} asset(s))."
else
  log "Checked ${CHECKED} published asset(s) against registry.json for ${TAG}. Mismatches: ${MISMATCHES}."
fi

if [ "$MISSING_ASSETS" -gt 0 ]; then
  err "${MISSING_ASSETS} expected asset(s) are genuinely absent from release ${TAG}."
  err "That is a release defect: rebuild and upload the missing artifact(s)."
fi

if [ "$DOWNLOAD_ERRORS" -gt 0 ]; then
  err "${DOWNLOAD_ERRORS} asset(s) are published but could not be downloaded."
  err "That is transient (network or GitHub rate limiting), NOT a missing artifact."
  err "Re-run this script; do not rebuild or re-upload anything on account of it."
fi

if [ "$MISMATCHES" -gt 0 ] || [ "$MISSING_ASSETS" -gt 0 ] || [ "$DOWNLOAD_ERRORS" -gt 0 ]; then
  exit 1
fi

log "Done."
