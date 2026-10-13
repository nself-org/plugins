#!/usr/bin/env bash
# verify-signatures.sh --pub FILE DIR
#
# Purpose: check every *.tar.gz in DIR against DIR/releases.json and against exactly one public key, the way an installer
#   must (hq PLUG D7, contract plugins.release-signature v1): checksum first, then signature, both before anything is extracted.
#   There is no key discovery: the key is the one --pub names (PEM SPKI Ed25519), so a rehearsal signature verifies against the
#   rehearsal test key and fails against every other key, committed or not. A check that only compared sha256 to releases.json
#   would pass for any key; this one does not.
#
# Input: DIR holds tarballs and a releases.json in the release-data shape ({"releases": {"<slug>": {"version", "sha256",
#   "platform_checksums", "release_signature": {"key_id", "alg", "sig"}}}}). rehearse.sh writes it; P7-PLUG-69 copies the
#   published one. A tarball is matched to its row by name: <slug>-<version>.tar.gz is the source tarball (sha256 + signature),
#   <slug>-<version>-<platform>.tar.gz is a binary tarball (platform_checksums only: the signature covers the source tarball).
#
# Output (stderr, exit 1 on any failure; nothing on stdout but the summary), per tarball, in this order:
#   checksum mismatch: <file>               sha256 of the file differs from releases.json
#   missing release data: <file>            no releases.json row (or no checksum) for the file
#   unsigned: <slug> <version>              the row has no release_signature
#   signature mismatch: <slug> <version>    bad alg, key id other than the one derived from --pub, or Ed25519 verification failed
# Exit 0 only when every tarball passes. Exit 2 on usage errors.
#
# Requires jq and OpenSSL 3 (LibreSSL cannot verify Ed25519 -rawin); set OPENSSL to pick the binary.

set -euo pipefail

err() { printf "%s\n" "$*" >&2; }

PUB=""
if [ "${1:-}" = "--pub" ]; then
  PUB="${2:-}"
  shift 2 || true
fi
DIR="${1:-}"
if [ -z "$PUB" ] || [ -z "$DIR" ] || [ "$#" -ne 1 ]; then
  err "usage: $0 --pub PUBLIC_KEY.pem DIR"
  exit 2
fi
[ -f "$PUB" ] || { err "public key not found: $PUB"; exit 2; }
[ -d "$DIR" ] || { err "directory not found: $DIR"; exit 2; }
RELEASES="$DIR/releases.json"
[ -f "$RELEASES" ] || { err "missing release data: $RELEASES"; exit 1; }
command -v jq >/dev/null 2>&1 || { err "jq not found"; exit 1; }
O="${OPENSSL:-openssl}"
command -v "$O" >/dev/null 2>&1 || { err "openssl not found (set OPENSSL to an OpenSSL 3 binary)"; exit 1; }
"$O" version 2>/dev/null | grep -q '^OpenSSL ' || { err "$O is not OpenSSL (LibreSSL cannot verify Ed25519 -rawin); set OPENSSL"; exit 1; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/verify-sig.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# The key id is derived from the public key exactly as cli sdk/go/signing KeyID does: plugins-<16 hex of sha256(raw 32 bytes)>.
EXPECTED_ID="plugins-$("$O" pkey -pubin -in "$PUB" -outform DER 2>/dev/null | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)"
"$O" pkey -pubin -in "$PUB" -noout 2>/dev/null || { err "not a PEM public key: $PUB"; exit 2; }

# Row lookup by file name: one jq pass writes "file<TAB>kind<TAB>slug<TAB>version<TAB>platform" for every known asset name.
jq -r '.releases | to_entries[] | .key as $s | .value as $v | select($v.version != null) |
  ($s + "-" + $v.version + ".tar.gz") as $src |
  [$src, "source", $s, $v.version, ""],
  (($v.platform_checksums // {}) | keys[] | [($s + "-" + $v.version + "-" + . + ".tar.gz"), "platform", $s, $v.version, .])
  | @tsv' "$RELEASES" > "$WORK/index.tsv"

fail=0
checked=0
shopt -s nullglob
for f in "$DIR"/*.tar.gz; do
  name="$(basename "$f")"
  line="$(awk -F'\t' -v n="$name" '$1 == n { print; exit }' "$WORK/index.tsv")"
  if [ -z "$line" ]; then
    err "missing release data: $name"
    fail=1
    continue
  fi
  IFS=$'\t' read -r _ kind slug version platform <<<"$line"
  checked=$((checked + 1))
  actual="$(sha256_file "$f")"

  if [ "$kind" = "platform" ]; then
    want="$(jq -r --arg s "$slug" --arg p "$platform" '.releases[$s].platform_checksums[$p] // ""' "$RELEASES")"
  else
    want="$(jq -r --arg s "$slug" '.releases[$s].sha256 // ""' "$RELEASES")"
  fi
  if [ -z "$want" ]; then
    err "missing release data: $name"
    fail=1
    continue
  fi
  if [ "$actual" != "$want" ]; then
    err "checksum mismatch: $name"
    fail=1
    continue
  fi
  [ "$kind" = "source" ] || continue

  sig_json="$(jq -c --arg s "$slug" '.releases[$s].release_signature // empty' "$RELEASES")"
  if [ -z "$sig_json" ]; then
    err "unsigned: $slug $version"
    fail=1
    continue
  fi
  alg="$(jq -r '.alg // ""' <<<"$sig_json")"
  key_id="$(jq -r '.key_id // ""' <<<"$sig_json")"
  sig="$(jq -r '.sig // ""' <<<"$sig_json")"
  if [ "$alg" != "ed25519" ] || [ "$key_id" != "$EXPECTED_ID" ] || [ -z "$sig" ]; then
    err "signature mismatch: $slug $version"
    fail=1
    continue
  fi
  printf 'nself-plugin-v1\n%s\n%s\n%s\n' "$slug" "$version" "$actual" > "$WORK/msg"
  if ! printf '%s' "$sig" | "$O" base64 -d -A > "$WORK/sig" 2>/dev/null || [ "$(wc -c < "$WORK/sig" | tr -d ' ')" != "64" ] ||
     ! "$O" pkeyutl -verify -rawin -pubin -inkey "$PUB" -in "$WORK/msg" -sigfile "$WORK/sig" >/dev/null 2>&1; then
    err "signature mismatch: $slug $version"
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  exit 1
fi
if [ "$checked" -eq 0 ]; then
  err "missing release data: no tarballs in $DIR"
  exit 1
fi
printf "verified %s tarball(s) against %s (%s)\n" "$checked" "$PUB" "$EXPECTED_ID"
