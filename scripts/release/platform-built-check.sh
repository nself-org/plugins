#!/usr/bin/env bash
# platform-built-check.sh BUILT_LIST DIR
#
# Purpose: the gate in front of platform (binary) signing in .github/workflows/release-tarballs.yml (contract plugins.release-signature
#   v1.1, ticket P7-PLUG-83). The publish job never uses --clobber, so a tarball already on the Release wins over this run's build.
#   Signing a platform tarball vouches for the binary inside it, so the sign job may only sign bytes THIS run built: anyone with
#   contents:write could otherwise upload a binary while the sign job waits for approval and receive the official signature.
#
# Inputs:
#   BUILT_LIST  sha256sum-format lines ("<64 hex>  <file name>") of the tarballs this run built and the Release serves unchanged
#               (the publish job writes it as built-sha256.txt). It may be empty. Lines in any other shape grant nothing.
#   DIR         a directory (symlinks are followed) holding the tarballs about to be signed. Only platform tarballs count:
#               names ending in -darwin-amd64 -darwin-arm64 -linux-amd64 -linux-arm64 -windows-amd64 .tar.gz.
#
# Output: exit 0 only if every platform tarball in DIR has its exact "<sha256>  <name>" line in BUILT_LIST (the name is part of the
#   match: the bytes of one platform renamed as another are refused too). Otherwise exit 1 and one stderr line per refused file:
#     not built by this run: <file>
#   Exit 2 on usage errors (a missing BUILT_LIST or DIR is an error, never an empty pass). Nothing is written.
#
# Requires sha256sum or shasum.

set -euo pipefail

err() { printf "[platform-built-check] %s\n" "$*" >&2; }

if [ "$#" -ne 2 ]; then
  err "usage: $0 BUILT_LIST DIR"
  exit 2
fi
LIST="$1"
DIR="$2"
[ -f "$LIST" ] || { err "built list not found: $LIST"; exit 2; }
[ -d "$DIR" ] || { err "directory not found: $DIR"; exit 2; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# "<sha>\t<name>" per well-formed line; text mode ("sha  name") and binary mode ("sha *name") both appear in the wild.
TAB=$'\t'
GRANTS="$(sed -nE "s/^([0-9a-f]{64}) [ *](.+)\$/\\1${TAB}\\2/p" "$LIST")"

fail=0
checked=0
shopt -s nullglob
for f in "$DIR"/*-darwin-amd64.tar.gz "$DIR"/*-darwin-arm64.tar.gz "$DIR"/*-linux-amd64.tar.gz "$DIR"/*-linux-arm64.tar.gz "$DIR"/*-windows-amd64.tar.gz; do
  name="$(basename "$f")"
  checked=$((checked + 1))
  # A name with a line break or tab could smuggle a granted "<sha><TAB><name>" line into the match below.
  case "$name" in
    *$'\n'*|*$'\t'*) err "not built by this run: refusing a file name with a line break or tab"; fail=1; continue ;;
  esac
  sha="$(sha256_file "$f")"
  if ! printf '%s\n' "$GRANTS" | grep -qxF -- "$(printf '%s\t%s' "$sha" "$name")"; then
    err "not built by this run: $name"
    fail=1
  fi
done
[ "$fail" -eq 0 ] || exit 1
printf "[platform-built-check] %s platform tarball(s) in %s were built by this run\n" "$checked" "$DIR"
