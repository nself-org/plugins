#!/usr/bin/env bash
# verify-published-checksums.sh
#
# Purpose: the release model this repo ships under (see .github/RELEASE-v1.2.1.md and tarball-checksum-gate.yml) treats the
#   PUBLISHED release asset as the artifact of record. Checksums are written FROM those published bytes, not from an
#   independent local rebuild that merely hopes to match (gzip's DEFLATE output is implementation-dependent; see
#   tarball-checksum-gate.yml for the measured proof). Since P7-PLUG-36 the release data lives in releases.json and
#   registry.json is a generated projection of it (scripts/plugin-counts.sh), so this is the one script that writes
#   releases.json, and it reads real published bytes to do it.
#
# Inputs: TAG (e.g. v1.2.7), a release that must already exist (a draft works with repo access). releases.json in the repo
#   root is read and, with --write, rewritten. A row is checked when its release_tag equals TAG. With --write, a free
#   plugin whose manifest version equals TAG's version, whose source tarball <name>-<version>.tar.gz is on the release and
#   whose row is missing or points at another tag, also gets its row written (this is how a new release lands).
#
# For each such plugin this downloads the source tarball and, when the plugin's manifest declares a binaryName (or its row
#   already has platform_checksums), the five per-platform binary tarballs <name>-<version>-<platform>.tar.gz, and computes
#   sha256 for each.
#
# Outputs:
#   Default (report mode): prints ONLY the mismatches found (releases.json value vs the freshly computed hash) and a final
#     summary line; nothing is modified. Exit 0 if no mismatches, exit 1 if any (a value disagreed or an expected asset was
#     missing from the release).
#   --write: writes version, sha256, release_tag, tarball_url, platform_checksums and binaries (the nself-* files in the
#     linux-amd64 tarball) into releases.json, keeping an existing release_signature, then tells you to run
#     scripts/plugin-counts.sh to regenerate registry.json. Exit 0 on success, exit 1 if any expected asset could not be
#     downloaded (releases.json is not written then).
#
# Constraints: requires `gh` (authenticated, repo access), `jq`, `tar`, and `sha256sum`/`shasum`. Never creates or publishes
#   a release, never tags, never touches anything but releases.json.
#
# Usage:
#   ./scripts/verify-published-checksums.sh v1.2.7
#   ./scripts/verify-published-checksums.sh --write v1.2.7

set -euo pipefail

REPO="nself-org/plugins"
WRITE=false
if [ "${1:-}" = "--write" ]; then
  WRITE=true
  shift
fi
TAG="${1:-}"
if [ -z "$TAG" ]; then
  printf "Usage: %s [--write] TAG\n  e.g. %s v1.2.7\n       %s --write v1.2.7\n" "$0" "$0" "$0" >&2
  exit 1
fi
VERSION="${TAG#v}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
RELEASES_FILE="${REPO_ROOT}/releases.json"
PLUGINS_DIR="${REPO_ROOT}/free"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/verify-checksums.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT

# Matches internal/plugin/arch.go's PlatformArch(), the same list scripts/build-tarballs.sh cross-compiles for a binaryName plugin.
PLATFORMS="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64"
BINARY_RE='^nself-[a-z][a-z0-9-]*$'

log() { printf "[verify-published-checksums] %s\n" "$*"; }
err() { printf "[verify-published-checksums] ERROR: %s\n" "$*" >&2; }

for tool in gh jq tar; do
  command -v "$tool" >/dev/null 2>&1 || { err "$tool not found."; exit 1; }
done
[ -f "$RELEASES_FILE" ] || { err "releases.json not found at ${RELEASES_FILE} (run scripts/plugin-counts.sh --init-releases once)"; exit 1; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

if ! gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  err "Release $TAG not found in $REPO (draft releases work fine with repo access — create it first)."
  exit 1
fi

RELEASES_JSON="$(cat "$RELEASES_FILE")"
MISMATCHES=0
MISSING_ASSETS=0
DOWNLOAD_ERRORS=0
CHECKED=0
WRITTEN=0

# Fetch the release's asset list ONCE, and fetch the bytes ONCE. Per-asset `gh release download` calls trip GitHub's
# secondary rate limit and, with the output discarded, made a throttled download look like a MISSING asset (a false alarm
# seen on v1.2.1 for flags/linux-arm64). So: list once, download everything once, then work from local files. An asset absent
# from the list is MISSING (a real release defect); one in the list with no local file is a DOWNLOAD ERROR (transient).
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

# Plugins to process, in a stable order: rows already on TAG, plus (with --write) manifests at this version with no row on TAG.
candidates="$(jq -r --arg t "$TAG" '.releases | to_entries[] | select(.value.release_tag == $t) | .key' <<<"$RELEASES_JSON")"
if [ "$WRITE" = "true" ]; then
  for dir in "$PLUGINS_DIR"/*/; do
    name="$(basename "$dir")"
    [ -f "$dir/plugin.json" ] || continue
    [ "$(jq -r '.version // ""' "$dir/plugin.json")" = "$VERSION" ] || continue
    asset_is_published "${name}-${VERSION}.tar.gz" || continue
    candidates="$(printf '%s\n%s' "$candidates" "$name")"
  done
fi

while IFS= read -r name; do
  [ -n "$name" ] || continue
  row="$(jq -c --arg n "$name" '.releases[$n] // {}' <<<"$RELEASES_JSON")"
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

  if [ "$WRITE" != "true" ]; then
    reg_sha="$(jq -r '.sha256 // ""' <<<"$row")"
    if [ -n "$reg_sha" ] && [ "$reg_sha" != "$computed_sha" ]; then
      printf "MISMATCH %s: sha256=%s  published=%s\n" "$name" "$reg_sha" "$computed_sha"
      MISMATCHES=$((MISMATCHES + 1))
    fi
  fi

  # Per-platform assets: only for plugins that ship a binary (the manifest declares a binaryName, or the row already has platforms).
  bin_name=""
  [ -f "${PLUGINS_DIR}/${name}/plugin.json" ] && bin_name="$(jq -r '.binaryName // .implementation.binaryName // ""' "${PLUGINS_DIR}/${name}/plugin.json" 2>/dev/null || printf '')"
  has_platforms="$(jq -r '(.platform_checksums // {}) | length > 0' <<<"$row")"
  platforms_json="{}"
  if [ -n "$bin_name" ] || [ "$has_platforms" = "true" ]; then
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
      platforms_json="$(jq -c --arg p "$platform" --arg s "$p_computed" '.[$p] = $s' <<<"$platforms_json")"
      if [ "$WRITE" != "true" ]; then
        reg_platform="$(jq -r --arg p "$platform" '.platform_checksums[$p] // ""' <<<"$row")"
        if [ -n "$reg_platform" ] && [ "$reg_platform" != "$p_computed" ]; then
          printf "MISMATCH %s/%s: platform_checksums.%s=%s  published=%s\n" "$name" "$platform" "$platform" "$reg_platform" "$p_computed"
          MISMATCHES=$((MISMATCHES + 1))
        fi
      fi
    done
  fi

  if [ "$WRITE" = "true" ]; then
    # The binaries a released CLI requires of the archive: the nself-* files in the linux-amd64 tarball.
    binaries_json="[]"
    linux_tar="${WORK_DIR}/${name}-${VERSION}-linux-amd64.tar.gz"
    if [ -f "$linux_tar" ]; then
      binaries_json="$( { tar -tzf "$linux_tar" | awk -F/ '{print $NF}' | grep -E "$BINARY_RE" || true; } | sort -u | jq -R . | jq -sc .)"
      [ "$binaries_json" != "[]" ] || log "WARNING: ${name}-${VERSION}-linux-amd64.tar.gz holds no nself-* binary; the row gets no binaries list"
    fi
    RELEASES_JSON="$(jq --arg n "$name" --arg v "$VERSION" --arg tag "$TAG" --arg sha "$computed_sha" \
      --arg url "https://github.com/${REPO}/releases/download/${TAG}/${tarball_name}" \
      --argjson plat "$platforms_json" --argjson bins "$binaries_json" '
      .releases[$n] = ((.releases[$n] // {release_signature: null})
        | (if .sha256 != $sha then .release_signature = null else . end)
        | . + {version: $v, sha256: $sha, release_tag: $tag, tarball_url: $url}
        | del(.tarball)
        | if ($plat | length) > 0 then .platform_checksums = $plat else del(.platform_checksums) end
        | if ($bins | length) > 0 then .binaries = $bins else del(.binaries) end)' <<<"$RELEASES_JSON")"
    WRITTEN=$((WRITTEN + 1))
  fi
done <<<"$(printf '%s\n' "$candidates" | sort -u)"

if [ "$MISSING_ASSETS" -gt 0 ] || [ "$DOWNLOAD_ERRORS" -gt 0 ]; then
  [ "$WRITE" != "true" ] || log "releases.json NOT written: an expected asset is missing or failed to download."
elif [ "$WRITE" = "true" ]; then
  jq -S --indent 2 . <<<"$RELEASES_JSON" > "${RELEASES_FILE}.tmp" && mv "${RELEASES_FILE}.tmp" "$RELEASES_FILE"
  log "Wrote ${WRITTEN} row(s) into releases.json from ${TAG}'s published assets (checked ${CHECKED} asset(s))."
  log "Next: scripts/plugin-counts.sh regenerates registry.json, catalog.json and counts.json from it."
else
  log "Checked ${CHECKED} published asset(s) against releases.json for ${TAG}. Mismatches: ${MISMATCHES}."
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
