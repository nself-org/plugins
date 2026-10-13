#!/usr/bin/env bash
# build-and-upload-tarballs.sh
# Builds a tarball + sha256 checksum for each free plugin and uploads them
# to the nself-org/plugins GitHub Release for a given tag.
#
# Usage:
#   ./scripts/build-and-upload-tarballs.sh [TAG]
#   TAG defaults to v1.0.0
#
# Requirements: gh CLI authenticated, jq, sha256sum (or shasum on macOS)
# Idempotent: re-running skips assets already present on the release.

set -euo pipefail

REPO="nself-org/plugins"
TAG="${1:-v1.0.0}"
PLUGINS_DIR="free"
WORK_DIR="$(mktemp -d)"
BATCH_SIZE=10
ERRORS=0

cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

log() { printf "[build-tarballs] %s\n" "$*"; }
err() { printf "[build-tarballs] ERROR: %s\n" "$*" >&2; ERRORS=$((ERRORS + 1)); }

# sha256 helper — macOS uses shasum -a 256, Linux uses sha256sum
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# Check prerequisites
if ! command -v gh >/dev/null 2>&1; then
  err "gh CLI not found. Install from https://cli.github.com"
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  err "jq not found. Install jq."
  exit 1
fi

# Verify release exists
if ! gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  err "Release $TAG not found in $REPO. Create the release first."
  exit 1
fi

# Fetch already-uploaded asset names (idempotency)
EXISTING_ASSETS=$(gh release view "$TAG" --repo "$REPO" --json assets --jq '[.assets[].name]' 2>/dev/null || printf "[]")
log "Existing assets on $TAG: $(printf '%s' "$EXISTING_ASSETS" | jq 'length')"

# Collect plugin directories
PLUGIN_DIRS=()
for plugin_dir in "$PLUGINS_DIR"/*/; do
  [ -d "$plugin_dir" ] || continue
  PLUGIN_DIRS+=("$plugin_dir")
done

log "Found ${#PLUGIN_DIRS[@]} plugins to package"

# Build tarballs and checksums
BATCH=()

for plugin_dir in "${PLUGIN_DIRS[@]}"; do
  plugin_name="$(basename "$plugin_dir")"
  tarball_name="${plugin_name}-${TAG#v}.tar.gz"
  checksum_name="${tarball_name}.sha256"
  tarball_path="$WORK_DIR/$tarball_name"
  checksum_path="$WORK_DIR/$checksum_name"

  # Build tarball (from repo root so paths are stable)
  log "Building $tarball_name ..."
  if ! tar -czf "$tarball_path" "$plugin_dir" 2>/dev/null; then
    err "Failed to build tarball for $plugin_name"
    continue
  fi

  # Compute checksum
  sha256="$(sha256_file "$tarball_path")"
  printf "%s  %s\n" "$sha256" "$tarball_name" > "$checksum_path"

  # Skip upload if already present
  if printf '%s' "$EXISTING_ASSETS" | jq -e --arg n "$tarball_name" 'index($n) != null' >/dev/null 2>&1; then
    log "  Skipping $tarball_name (already uploaded)"
    continue
  fi
  if printf '%s' "$EXISTING_ASSETS" | jq -e --arg n "$checksum_name" 'index($n) != null' >/dev/null 2>&1; then
    log "  Skipping $checksum_name (already uploaded)"
    BATCH+=("$tarball_path")
    continue
  fi

  BATCH+=("$tarball_path" "$checksum_path")

  # Upload in batches of BATCH_SIZE
  if [ "${#BATCH[@]}" -ge "$((BATCH_SIZE * 2))" ]; then
    log "Uploading batch of ${#BATCH[@]} files ..."
    if ! gh release upload "$TAG" "${BATCH[@]}" --repo "$REPO" --clobber; then
      err "Batch upload failed for some assets"
    fi
    BATCH=()
  fi
done

# Upload remaining batch
if [ "${#BATCH[@]}" -gt 0 ]; then
  log "Uploading final batch of ${#BATCH[@]} files ..."
  if ! gh release upload "$TAG" "${BATCH[@]}" --repo "$REPO" --clobber; then
    err "Final batch upload failed for some assets"
  fi
fi

# registry.json is a generated projection (scripts/plugin-counts.sh) and releases.json is written only from published bytes:
# after the upload, run `scripts/verify-published-checksums.sh --write "$TAG"` and then `scripts/plugin-counts.sh`.
log "registry.json and releases.json are not written here; run scripts/verify-published-checksums.sh --write $TAG next"

# Verify
log "Verifying upload ..."
UPLOADED=$(gh release view "$TAG" --repo "$REPO" --json assets --jq '.assets | length' 2>/dev/null || printf "0")
log "Total assets now on $TAG: $UPLOADED"

if [ "$ERRORS" -gt 0 ]; then
  log "Completed with $ERRORS error(s). Check output above."
  exit 1
fi

log "Done. $UPLOADED assets on release $TAG."
