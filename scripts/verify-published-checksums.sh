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
#   --signatures FILE (with --write): the plugins to write are exactly the signed ones (plus rows already on TAG), instead of the
#     manifest-version scan. FILE is the release workflow's signing output, {"<slug>": {"sha256", "release_signature"}}
#     (see scripts/release/sign.sh). A row's release_signature is taken from it only when its sha256 equals the freshly
#     downloaded published bytes; a row with no matching signature is UNSIGNED and, like a missing asset, stops the write (exit 1),
#     so a release can never land half-signed. Without the flag an existing release_signature is kept (nulled if the bytes changed).
#
# Constraints: requires `gh` (authenticated, repo access), `jq`, `tar`, and `sha256sum`/`shasum`. Never creates or publishes
#   a release, never tags, never touches anything but releases.json.
#
# Usage:
#   ./scripts/verify-published-checksums.sh v1.2.7
#   ./scripts/verify-published-checksums.sh --write v1.2.7
#   ./scripts/verify-published-checksums.sh --write --signatures signatures.json v1.2.7

set -euo pipefail

REPO="nself-org/plugins"
WRITE=false
SIGNATURES_FILE=""
while [ "${1:-}" = "--write" ] || [ "${1:-}" = "--signatures" ]; do
  if [ "$1" = "--write" ]; then
    WRITE=true
    shift
  else
    SIGNATURES_FILE="${2:-}"
    shift 2 || shift $#
  fi
done
TAG="${1:-}"
if [ -z "$TAG" ]; then
  printf "Usage: %s [--write [--signatures FILE]] TAG\n  e.g. %s v1.2.7\n       %s --write v1.2.7\n" "$0" "$0" "$0" >&2
  exit 1
fi
if [ -n "$SIGNATURES_FILE" ] && { [ "$WRITE" != "true" ] || [ ! -f "$SIGNATURES_FILE" ]; }; then
  printf "[verify-published-checksums] ERROR: --signatures needs --write and an existing file\n" >&2
  exit 1
fi
VERSION="${TAG#v}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=scripts/conformance/lib.sh
source "${SCRIPT_DIR}/conformance/lib.sh"
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

# version_is_newer A B: A is a strictly newer release than B by SemVer precedence (numeric core; a final release beats its prereleases;
# prerelease identifiers left to right: numbers below words, numbers numerically, words by bytes, shorter list below longer; +build ignored).
version_is_newer() {
  awk -v a="$1" -v b="$2" '
    function isnum(s) { return s ~ /^[0-9]+$/ }
    function parse(v, out,   i) { sub(/\+.*/, "", v); i = index(v, "-"); out["core"] = i ? substr(v, 1, i - 1) : v; out["pre"] = i ? substr(v, i + 1) : "" }
    function cmp(x, y,   X, Y, xs, ys, nx, ny, n, i, p, q) {
      parse(x, X); parse(y, Y); nx = split(X["core"], xs, "."); ny = split(Y["core"], ys, "."); n = (nx > ny) ? nx : ny
      for (i = 1; i <= n; i++) { p = (i <= nx) ? xs[i] + 0 : 0; q = (i <= ny) ? ys[i] + 0 : 0; if (p != q) return (p > q) ? 1 : -1 }
      if (X["pre"] == Y["pre"]) return 0
      if (X["pre"] == "" || Y["pre"] == "") return (X["pre"] == "") ? 1 : -1
      nx = split(X["pre"], xs, "."); ny = split(Y["pre"], ys, "."); n = (nx < ny) ? nx : ny
      for (i = 1; i <= n; i++) {
        if (xs[i] "" == ys[i] "") continue
        if (isnum(xs[i]) && isnum(ys[i])) { p = xs[i] + 0; q = ys[i] + 0; if (p != q) return (p > q) ? 1 : -1; continue }
        if (isnum(xs[i]) || isnum(ys[i])) return isnum(xs[i]) ? -1 : 1
        return ((xs[i] "") > (ys[i] "")) ? 1 : -1
      }
      return (nx == ny) ? 0 : ((nx > ny) ? 1 : -1)
    }
    BEGIN { exit !(cmp(a, b) > 0) }'
}

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
UNSIGNED=0
ROLLBACKS=0
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
# With --signatures the signed set decides instead: every signed source tarball must land as a row (or fail as MISSING), whatever
# version the default branch's manifests say by now.
candidates="$(jq -r --arg t "$TAG" '.releases | to_entries[] | select(.value.release_tag == $t) | .key' <<<"$RELEASES_JSON")"
if [ -n "$SIGNATURES_FILE" ]; then
  candidates="$(printf '%s\n%s' "$candidates" "$(jq -r 'keys[]' "$SIGNATURES_FILE")")"
elif [ "$WRITE" = "true" ]; then
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
  # Only an active cli-targets exception permits a signed source-only release of a binary plugin.
  if [ -n "$SIGNATURES_FILE" ] && [ -n "$bin_name" ] && [ "$has_platforms" != "true" ]; then
    any_platform=false
    for platform in $PLATFORMS; do asset_is_published "${name}-${VERSION}-${platform}.tar.gz" && any_platform=true; done
    if [ "$any_platform" = "false" ] && exception_active "${SCRIPT_DIR}/conformance/exceptions.yaml" cli-targets "$name"; then
      log "NOTE: ${name} declares binaryName ${bin_name} but the release has no platform tarball; no binary is recorded"
      bin_name=""
    fi
  fi
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
  sig_json="null"
  if [ "$WRITE" = "true" ] && [ -n "$SIGNATURES_FILE" ]; then
    # A run for an older tag must not roll a newer row back (the manifest-version guard is off in this mode).
    row_version="$(jq -r '.version // ""' <<<"$row")"
    if [ -n "$row_version" ] && [ "$row_version" != "$VERSION" ] && version_is_newer "$row_version" "$VERSION"; then
      printf "ROLLBACK %s: releases.json already holds %s, newer than %s\n" "$name" "$row_version" "$VERSION"
      ROLLBACKS=$((ROLLBACKS + 1))
      continue
    fi
    sig_json="$(jq -c --arg n "$name" --arg sha "$computed_sha" '.[$n] | select(. != null and .sha256 == $sha) | .release_signature // empty' "$SIGNATURES_FILE")"
    if [ -z "$sig_json" ]; then
      printf "UNSIGNED %s: %s has no signature for the published sha256 %s\n" "$name" "$SIGNATURES_FILE" "$computed_sha"
      UNSIGNED=$((UNSIGNED + 1))
      continue
    fi
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
      --argjson plat "$platforms_json" --argjson bins "$binaries_json" --argjson sig "$sig_json" '
      .releases[$n] = ((.releases[$n] // {release_signature: null})
        | (if .sha256 != $sha then .release_signature = null else . end)
        | (if $sig != null then .release_signature = $sig else . end)
        | . + {version: $v, sha256: $sha, release_tag: $tag, tarball_url: $url}
        | del(.tarball)
        | if ($plat | length) > 0 then .platform_checksums = $plat else del(.platform_checksums) end
        | if ($bins | length) > 0 then .binaries = $bins else del(.binaries) end)' <<<"$RELEASES_JSON")"
    WRITTEN=$((WRITTEN + 1))
  fi
done <<<"$(printf '%s\n' "$candidates" | sort -u)"

if [ "$MISSING_ASSETS" -gt 0 ] || [ "$DOWNLOAD_ERRORS" -gt 0 ] || [ "$UNSIGNED" -gt 0 ] || [ "$ROLLBACKS" -gt 0 ]; then
  [ "$WRITE" != "true" ] || log "releases.json NOT written: an expected asset is missing, failed to download or is unsigned."
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
if [ "$UNSIGNED" -gt 0 ]; then
  err "${UNSIGNED} row(s) have no signature over the published bytes; the signing job must sign what is on the release."
fi
if [ "$ROLLBACKS" -gt 0 ]; then
  err "${ROLLBACKS} row(s) already hold a newer version than ${TAG}; releases.json is not rolled back."
fi
if [ "$MISMATCHES" -gt 0 ] || [ "$MISSING_ASSETS" -gt 0 ] || [ "$DOWNLOAD_ERRORS" -gt 0 ] || [ "$UNSIGNED" -gt 0 ] || [ "$ROLLBACKS" -gt 0 ]; then
  exit 1
fi
log "Done."
