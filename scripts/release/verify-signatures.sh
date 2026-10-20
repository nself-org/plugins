#!/usr/bin/env bash
# verify-signatures.sh --pub FILE DIR
#
# Purpose: check every *.tar.gz in DIR against DIR/releases.json and against exactly one public key, the way an installer
#   must (hq PLUG D7, contract plugins.release-signature v1.1): checksum first, then signature, both before anything is extracted.
#   There is no key discovery: the key is the one --pub names (PEM SPKI Ed25519), so a rehearsal signature verifies against the
#   rehearsal test key and fails against every other key, committed or not. A check that only compared sha256 to releases.json
#   would pass for any key; this one does not.
#
# Input: DIR holds tarballs and a releases.json in the release-data shape ({"releases": {"<slug>": {"version", "sha256",
#   "platform_checksums", "release_signature": {"key_id", "alg", "sig"}, "platform_signatures": {"<platform>": {"key_id",
#   "alg", "sig"}}}}}). rehearse.sh writes it; P7-PLUG-69 copies the published one. A tarball is matched to its row by name:
#   <slug>-<version>.tar.gz is the source tarball (sha256 + signature over "nself-plugin-v1\n<slug>\n<version>\n<sha>\n"),
#   <slug>-<version>-<platform>.tar.gz is a binary tarball (sha256 in platform_checksums + signature in platform_signatures over
#   "nself-plugin-bin-v1\n<slug>\n<version>\n<platform>\n<sha>\n"). On a signed row (non-null release_signature) a binary
#   tarball with a checksum but no platform signature is a failure. The summary line counts the two kinds apart.
#
# Output (stderr, exit 1 on any failure; nothing on stdout but the summary), per tarball, in this order:
#   checksum mismatch: <file>               sha256 of the file differs from releases.json
#   missing release data: <file>            no releases.json row (or no checksum) for the file
#   missing tarball: <file>                 releases.json names a tarball that is not in DIR (a dropped file never passes)
#   unsigned: <slug> <version>              the row has no release_signature
#   signature mismatch: <slug> <version>    bad alg, key id other than the one derived from --pub, or Ed25519 verification failed
#   unsigned platform: <slug> <version> <platform>             a signed row has a binary tarball but no platform signature for it
#   platform signature mismatch: <slug> <version> <platform>   the platform signature is malformed, made by another key, or does not
#                                                              verify for this slug, version, platform and these bytes
#   orphan platform signature: <slug> <version> <platform>     platform_signatures names a platform with no platform_checksums entry,
#                                                              or sits on a row with no release_signature
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
PUB_TEXT="$("$O" pkey -pubin -in "$PUB" -noout -text 2>/dev/null)" || { err "not a PEM public key: $PUB"; exit 2; }
case "$PUB_TEXT" in "ED25519 Public-Key:"*) ;; *) err "not an Ed25519 public key: $PUB"; exit 2 ;; esac
EXPECTED_ID="plugins-$("$O" pkey -pubin -in "$PUB" -outform DER 2>/dev/null | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)"

# sig_check FILTER SLUG PLATFORM MSGFILE: succeeds when the object FILTER selects from releases.json is {alg ed25519, key_id of --pub,
# canonical 64-byte base64 sig} and the signature verifies over MSGFILE with --pub. jq compares the exact strings (a trailing newline in
# key_id or alg is a different string) and reads sig with -j so nothing is trimmed. sig must be canonical base64 of 64 bytes: 86
# characters and "==", the same text cli sdk/go/signing EncodeSig writes and DecodeSig accepts.
sig_check() {
  local sig
  sig="$(jq -j --arg s "$2" --arg p "$3" --arg id "$EXPECTED_ID" "try ($1) catch null"'
    | select(type == "object" and .alg == "ed25519" and .key_id == $id and (.sig | type) == "string" and (.sig | test("\\A[A-Za-z0-9+/]{86}==\\z")))
    | .sig' "$RELEASES")" || return 1
  [ -n "$sig" ] || return 1
  printf '%s' "$sig" | "$O" base64 -d -A > "$WORK/sig" 2>/dev/null || return 1
  [ "$(wc -c < "$WORK/sig" | tr -d ' ')" = "64" ] || return 1
  [ "$("$O" base64 -A -in "$WORK/sig")" = "$sig" ] || return 1
  "$O" pkeyutl -verify -rawin -pubin -inkey "$PUB" -in "$4" -sigfile "$WORK/sig" >/dev/null 2>&1
}

# Row lookup by file name: one jq pass writes "file<TAB>kind<TAB>slug<TAB>version<TAB>platform" for every known asset name.
# Only the five published platforms count as binary tarballs, and a file name may appear once: a name that two rows could claim
# is ambiguous release data, never a way to be checked as the weaker kind.
jq -r '.releases | to_entries[] | .key as $s | .value as $v | select($v.version != null) |
  ($s + "-" + $v.version + ".tar.gz") as $src |
  [$src, "source", $s, $v.version, ""],
  (($v.platform_checksums // {}) | keys[] | select(. as $p | ["darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64", "windows-amd64"] | index($p))
    | [($s + "-" + $v.version + "-" + . + ".tar.gz"), "platform", $s, $v.version, .])
  | @tsv' "$RELEASES" > "$WORK/index.tsv"
dups="$(cut -f1 "$WORK/index.tsv" | sort | uniq -d)"
if [ -n "$dups" ]; then
  err "ambiguous release data: more than one row claims $(printf '%s' "$dups" | tr '\n' ' ')"
  exit 1
fi

fail=0
checked=0
source_ok=0
platform_ok=0

# A platform signature must sit on a signed row and name a platform the row has a checksum for; anything else is not a release this
# producer writes (a signature "for" bytes nothing records is never a pass). Rows with a non-object platform_signatures are orphans too.
while IFS=$'\t' read -r o_slug o_version o_platform; do
  [ -n "$o_slug" ] || continue
  err "orphan platform signature: $o_slug $o_version $o_platform"
  fail=1
done < <(jq -r '.releases | to_entries[] | .key as $s | .value as $v | select($v.version != null and ($v | has("platform_signatures")))
  | ($v.platform_signatures | if type == "object" then keys else ["(not an object)"] end)[] as $p
  | select(($v.release_signature | type) != "object" or (($v.platform_checksums // {}) | has($p) | not)) | [$s, $v.version, $p] | @tsv' "$RELEASES")
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
  if [ "$kind" = "platform" ]; then
    # A row without a source signature fails on its source tarball ("unsigned"), so its binary tarballs are not reported again here.
    if [ "$(jq -r --arg s "$slug" '.releases[$s].release_signature | type' "$RELEASES")" = "null" ]; then
      continue
    fi
    if [ "$(jq -r --arg s "$slug" --arg p "$platform" 'try (.releases[$s].platform_signatures[$p] | type) catch "null"' "$RELEASES")" = "null" ]; then
      err "unsigned platform: $slug $version $platform"
      fail=1
      continue
    fi
    printf 'nself-plugin-bin-v1\n%s\n%s\n%s\n%s\n' "$slug" "$version" "$platform" "$actual" > "$WORK/msg"
    if ! sig_check '.releases[$s].platform_signatures[$p]' "$slug" "$platform" "$WORK/msg"; then
      err "platform signature mismatch: $slug $version $platform"
      fail=1
      continue
    fi
    platform_ok=$((platform_ok + 1))
    continue
  fi

  if [ "$(jq -r --arg s "$slug" '.releases[$s].release_signature | type' "$RELEASES")" = "null" ]; then
    err "unsigned: $slug $version"
    fail=1
    continue
  fi
  printf 'nself-plugin-v1\n%s\n%s\n%s\n' "$slug" "$version" "$actual" > "$WORK/msg"
  if ! sig_check '.releases[$s].release_signature' "$slug" "" "$WORK/msg"; then
    err "signature mismatch: $slug $version"
    fail=1
    continue
  fi
  source_ok=$((source_ok + 1))
done

# Every tarball the release data names must be present: removing a signed file from DIR is a failure, not a smaller set.
while IFS=$'\t' read -r name _; do
  [ -f "$DIR/$name" ] || { err "missing tarball: $name"; fail=1; }
done < "$WORK/index.tsv"

if [ "$fail" -ne 0 ]; then
  exit 1
fi
if [ "$checked" -eq 0 ]; then
  err "missing release data: no tarballs in $DIR"
  exit 1
fi
printf "verified %s source tarball(s) and %s binary tarball(s) (checksum + signature each) against %s (%s)\n" "$source_ok" "$platform_ok" "$PUB" "$EXPECTED_ID"
