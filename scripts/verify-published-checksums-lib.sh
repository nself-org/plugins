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
asset_is_published() { grep -Fxq "$1" "$ASSET_MANIFEST"; }

# Matches internal/plugin/arch.go's PlatformArch(), the same list scripts/build-tarballs.sh cross-compiles for a binaryName plugin.
PLATFORMS="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64"
BINARY_RE='^nself-[a-z][a-z0-9-]*$'
