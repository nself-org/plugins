#!/usr/bin/env bash
# build-tarballs.sh
# Build tarballs for all 27 free plugins without uploading.
# Outputs to dist/ directory. Run build-and-upload-tarballs.sh to also upload.
#
# Usage: ./scripts/build-tarballs.sh [VERSION]
#   VERSION defaults to the registry.json version field (e.g. 1.0.0)
#
# Requirements: jq, sha256sum (or shasum on macOS)
# Output: dist/<name>-<version>.tar.gz + dist/<name>-<version>.tar.gz.sha256

set -euo pipefail

PLUGINS_DIR="${PLUGINS_DIR:-free}"
DIST_DIR="${DIST_DIR:-dist}"
ERRORS=0

log()  { printf "[build-tarballs] %s\n" "$*"; }
err()  { printf "[build-tarballs] ERROR: %s\n" "$*" >&2; ERRORS=$((ERRORS + 1)); }
warn() { printf "[build-tarballs] WARN: %s\n" "$*" >&2; }

# Detect version from registry.json or first arg
if [ -n "${1:-}" ]; then
  VERSION="$1"
else
  if ! command -v jq >/dev/null 2>&1; then
    err "jq not found; pass VERSION as first argument"
    exit 1
  fi
  VERSION="$(jq -r '.version' registry.json 2>/dev/null || printf '')"
  if [ -z "$VERSION" ]; then
    err "Cannot read version from registry.json; pass VERSION as first argument"
    exit 1
  fi
fi

# sha256 helper — macOS uses shasum -a 256, Linux uses sha256sum
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

mkdir -p "$DIST_DIR"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DETERMINISTIC_TAR="${SCRIPT_DIR}/deterministic-tar.sh"

# Walk free/ plugin directories in a fixed, filesystem-order-independent
# sequence (matches the traversal .github/workflows/release-tarballs.yml
# uses) — member order inside each individual tarball is already pinned by
# deterministic-tar.sh's --sort=name, this just makes the build log/exit
# order predictable too.
BUILT=0
while IFS= read -r plugin_dir; do
  plugin_name="$(basename "$plugin_dir")"
  tarball_name="${plugin_name}-${VERSION}.tar.gz"
  checksum_name="${tarball_name}.sha256"
  tarball_path="${DIST_DIR}/${tarball_name}"
  checksum_path="${DIST_DIR}/${checksum_name}"

  # Verify plugin.json exists (skip non-plugin dirs)
  if [ ! -f "${plugin_dir}/plugin.json" ]; then
    warn "Skipping $plugin_name (no plugin.json)"
    continue
  fi

  log "Building ${tarball_name} ..."
  # No trailing slash on plugin_dir: deterministic-tar.sh's --sort=name
  # already makes member order stable regardless, but a consistent
  # no-trailing-slash argument keeps this script and the workflow's
  # `find -type d` output (which never has one) textually identical inputs.
  if ! "$DETERMINISTIC_TAR" "$tarball_path" "${plugin_dir%/}"; then
    err "Failed to build tarball for $plugin_name"
    continue
  fi

  sha256="$(sha256_file "$tarball_path")"
  printf "%s  %s\n" "$sha256" "$tarball_name" > "$checksum_path"

  log "  sha256: ${sha256}"
  BUILT=$((BUILT + 1))
done < <(find "$PLUGINS_DIR" -maxdepth 1 -mindepth 1 -type d | sort)

log "Built ${BUILT} tarballs in ${DIST_DIR}/"

if [ "$ERRORS" -gt 0 ]; then
  log "Completed with $ERRORS error(s)."
  exit 1
fi

log "Done."
