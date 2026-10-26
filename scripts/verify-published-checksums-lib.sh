#!/usr/bin/env bash
# Shared functions for verify-published-checksums.sh; sourced by the CLI.

# version_is_newer A B: A is a strictly newer release than B by SemVer precedence (numeric core; a final release beats its prereleases;
# prerelease identifiers left to right: numbers below words, numbers numerically, words by bytes, shorter list below longer; +build ignored).
# Digit strings are compared as strings (leading zeros dropped, longer wins, then byte order), never as awk floats: a double holds
# 15-16 digits, so two long identifiers that differ late would compare equal.
version_is_newer() {
  awk -v a="$1" -v b="$2" '
    function isnum(s) { return s ~ /^[0-9]+$/ }
    function ncmp(x, y) { sub(/^0+/, "", x); sub(/^0+/, "", y); x = x ""; y = y ""
      if (length(x) != length(y)) return (length(x) > length(y)) ? 1 : -1
      return (x == y) ? 0 : ((x > y) ? 1 : -1) }
    function parse(v, out,   i) { sub(/\+.*/, "", v); i = index(v, "-"); out["core"] = i ? substr(v, 1, i - 1) : v; out["pre"] = i ? substr(v, i + 1) : "" }
    function cmp(x, y,   X, Y, xs, ys, nx, ny, n, i, p, q, x1, y1) {
      parse(x, X); parse(y, Y); nx = split(X["core"], xs, "."); ny = split(Y["core"], ys, "."); n = (nx > ny) ? nx : ny
      for (i = 1; i <= n; i++) {
        x1 = (i <= nx) ? xs[i] : "0"; y1 = (i <= ny) ? ys[i] : "0"
        if (isnum(x1) && isnum(y1)) { p = ncmp(x1, y1); if (p != 0) return p; continue }
        p = x1 + 0; q = y1 + 0; if (p != q) return (p > q) ? 1 : -1
      }
      if (X["pre"] == Y["pre"]) return 0
      if (X["pre"] == "" || Y["pre"] == "") return (X["pre"] == "") ? 1 : -1
      nx = split(X["pre"], xs, "."); ny = split(Y["pre"], ys, "."); n = (nx < ny) ? nx : ny
      for (i = 1; i <= n; i++) {
        if (xs[i] "" == ys[i] "") continue
        if (isnum(xs[i]) && isnum(ys[i])) { p = ncmp(xs[i], ys[i]); if (p != 0) return p; continue }
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

log() { printf "[verify-published-checksums] %s\n" "$*"; }
err() { printf "[verify-published-checksums] ERROR: %s\n" "$*" >&2; }
# manifest_approves NAME SHA: the publish job's manifest (PUBLISHED_FILE, `sha256sum` lines) lists NAME with exactly SHA. True when no manifest is in use.
manifest_approves() {
  [ -n "${PUBLISHED_FILE:-}" ] || return 0
  awk -v n="$1" -v s="$2" '{ f = $2; sub(/^\*/, "", f); if ($1 == s && f == n) ok = 1 } END { exit !ok }' "$PUBLISHED_FILE"
}
asset_is_published() { grep -Fxq "$1" "$ASSET_MANIFEST"; }

# Matches internal/plugin/arch.go's PlatformArch(), the same list scripts/build-tarballs.sh cross-compiles for a binaryName plugin.
PLATFORMS="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64"
BINARY_RE='^nself-[a-z][a-z0-9-]*$'

# upsert_release_row NAME TARBALL_NAME SHA PLATFORMS_JSON BINARIES_JSON SIG_JSON PSIGS_JSON: print RELEASES_JSON with NAME's row rewritten
# from the published bytes (reads VERSION, TAG, REPO, SIGNATURES_FILE and RELEASES_JSON from the CLI).
upsert_release_row() {
  local name="$1" tarball_name="$2" computed_sha="$3" platforms_json="$4" binaries_json="$5" sig_json="$6" psigs_json="$7" signed=false
  [ -z "$SIGNATURES_FILE" ] || signed=true
  jq --arg n "$name" --arg v "$VERSION" --arg tag "$TAG" --arg sha "$computed_sha" \
    --arg url "https://github.com/${REPO}/releases/download/${TAG}/${tarball_name}" \
    --argjson plat "$platforms_json" --argjson bins "$binaries_json" --argjson sig "$sig_json" \
    --argjson psig "$psigs_json" --argjson signed "$signed" '
      (.releases[$n] // {release_signature: null}) as $old
      | .releases[$n] = ($old
        | (if .sha256 != $sha then .release_signature = null else . end)
        | (if $sig != null then .release_signature = $sig else . end)
        | . + {version: $v, sha256: $sha, release_tag: $tag, tarball_url: $url}
        | del(.tarball)
        | if ($plat | length) > 0 then .platform_checksums = $plat else del(.platform_checksums) end
        | .platform_signatures = (if $signed then $psig
            elif $old.version == $v and .release_signature != null then
              (($old.platform_signatures // {}) | with_entries(select(.key as $p | $plat[$p] != null and ($old.platform_checksums // {})[$p] == $plat[$p])))
            else {} end)
        | if (.platform_signatures | length) == 0 then del(.platform_signatures) else . end
        | if ($bins | length) > 0 then .binaries = $bins else del(.binaries) end)' <<<"$RELEASES_JSON"
}

# write_releases_json: write RELEASES_JSON over RELEASES_FILE atomically, or exit 1 with releases.json unchanged.
write_releases_json() {
  # `a && b` is exempt from errexit, so a failed write must be tested explicitly or the script would log Wrote and exit 0.
  # A unique temp file beside releases.json (same filesystem, so mv is atomic); cleanup is nonrecursive so nothing pre-existing is deleted.
  RELEASES_TMP=""
  if ! RELEASES_TMP="$(mktemp "${RELEASES_FILE}.XXXXXX")" || ! jq -S --indent 2 . <<<"$RELEASES_JSON" > "$RELEASES_TMP" || ! mv "$RELEASES_TMP" "$RELEASES_FILE"; then
    [ -z "$RELEASES_TMP" ] || rm -f "$RELEASES_TMP"
    err "could not write ${RELEASES_FILE}; releases.json is unchanged."
    exit 1
  fi
}

# report_failures: explain each non-zero failure counter of the CLI (MISSING_ASSETS, DOWNLOAD_ERRORS, UNSIGNED, UNAPPROVED, UNREADABLE, ROLLBACKS).
report_failures() {
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
  if [ "$UNAPPROVED" -gt 0 ]; then
    err "${UNAPPROVED} tarball(s) differ from the publish job's approved manifest; the Release changed after approval. Nothing is written."
  fi
  if [ "$UNREADABLE" -gt 0 ]; then
    err "${UNREADABLE} linux-amd64 archive(s) are published but corrupt; rebuild and upload them."
  fi
  if [ "$ROLLBACKS" -gt 0 ]; then
    err "${ROLLBACKS} row(s) already hold a newer version than ${TAG}; releases.json is not rolled back."
  fi
}

# fetch_release_assets: list TAG's assets into ASSET_MANIFEST and download them all into WORK_DIR, or exit 1 (see the CLI for why once).
fetch_release_assets() {
  local dl_err
  ASSET_MANIFEST="${WORK_DIR}/.asset-manifest"
  if ! gh release view "$TAG" --repo "$REPO" --json assets --jq '.assets[].name' > "$ASSET_MANIFEST" 2>/dev/null; then
    err "could not list assets on release $TAG"
    exit 1
  fi
  log "Release $TAG lists $(wc -l < "$ASSET_MANIFEST" | tr -d ' ') asset(s); downloading them once."

  dl_err="${WORK_DIR}/.download-stderr"
  if ! gh release download "$TAG" --repo "$REPO" --dir "$WORK_DIR" --clobber >/dev/null 2>"$dl_err"; then
    err "bulk download of $TAG's assets failed; cannot verify against published bytes."
    err "gh said: $(tr '\n' ' ' < "$dl_err" | cut -c1-400)"
    exit 1
  fi
  rm -f "$dl_err"
}
