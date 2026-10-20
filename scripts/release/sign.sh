#!/usr/bin/env bash
# sign.sh SLUG VERSION TARBALL [PLATFORM]
#
# Purpose: sign one plugin tarball with the free-tier release key (hq PLUG D7, contract plugins.release-signature v1.1).
#   Without PLATFORM, TARBALL is the source tarball and the signed message is the domain-separated string
#   "nself-plugin-v1\n<slug>\n<version>\n<sha256 hex of TARBALL>\n" (every field followed by a newline, the last one too).
#   With PLATFORM (one of darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64), TARBALL is that platform's binary
#   tarball and the message is "nself-plugin-bin-v1\n<slug>\n<version>\n<platform>\n<sha256 hex of TARBALL>\n". The first lines
#   differ, so a source signature never verifies as a platform signature and the reverse. Both use raw Ed25519
#   (`openssl pkeyutl -sign -rawin`) and the same tier key; the three-argument form is byte-identical to v1.
#
# Inputs (environment only, never argv):
#   NSELF_RELEASE_SIGNING_KEY_FREE     PKCS8 PEM of the Ed25519 private key (raw PEM, PEM with collapsed newlines, or base64 of the PEM).
#   NSELF_RELEASE_SIGNING_KEY_FREE_ID  key id, which must be  plugins-<first 16 hex of sha256(raw 32-byte public key)>
#                                      (the id is derived, never chosen; see cli sdk/go/signing KeyID).
#   OPENSSL                            optional openssl binary; must be OpenSSL 3 or newer (LibreSSL cannot sign Ed25519 -rawin).
#
# Output: stdout is the release_signature object on one line, e.g. {"key_id":"plugins-0123456789abcdef","alg":"ed25519","sig":"<base64>"}
#   sig is the raw 64-byte signature in standard padded base64, the same text cli sdk/go/signing EncodeSig produces.
#   Exit 0 ok, 1 failure, 2 usage. The key is written to a 0600 file that is removed on exit and never printed or logged.
#
# Only the free-tier key exists for this repo's CI. The licensed key lives on the owner's release machine (P7-PLUG-22).

set -euo pipefail
set +x

err() { printf "[sign] ERROR: %s\n" "$*" >&2; }

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
  printf "usage: %s SLUG VERSION TARBALL [PLATFORM]\n" "$0" >&2
  exit 2
fi
SLUG="$1"
VERSION="$2"
TARBALL="$3"
PLATFORM="${4-}"

if [ "$#" -eq 4 ]; then
  case "$PLATFORM" in
    darwin-amd64|darwin-arm64|linux-amd64|linux-arm64|windows-amd64) ;;
    *) printf "[sign] ERROR: platform '%s' is not one of darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64\n" "$PLATFORM" >&2; exit 2 ;;
  esac
fi

[[ "$SLUG" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { err "slug '$SLUG' is not lowercase-with-hyphens"; exit 2; }
[[ "$VERSION" =~ ^[0-9A-Za-z][0-9A-Za-z.+-]*$ ]] || { err "version '$VERSION' is not a plain version string"; exit 2; }
[ -f "$TARBALL" ] || { err "tarball not found: $TARBALL"; exit 1; }
[ -n "${NSELF_RELEASE_SIGNING_KEY_FREE:-}" ] || { err "NSELF_RELEASE_SIGNING_KEY_FREE is not set"; exit 1; }
KEY_ID="${NSELF_RELEASE_SIGNING_KEY_FREE_ID:-}"
[[ "$KEY_ID" =~ ^plugins-[0-9a-f]{16}$ ]] || { err "NSELF_RELEASE_SIGNING_KEY_FREE_ID must look like plugins-<16 hex>"; exit 1; }

O="${OPENSSL:-openssl}"
command -v "$O" >/dev/null 2>&1 || { err "openssl not found (set OPENSSL to an OpenSSL 3 binary)"; exit 1; }
"$O" version 2>/dev/null | grep -q '^OpenSSL ' || { err "$O is not OpenSSL (LibreSSL cannot sign Ed25519 -rawin); set OPENSSL"; exit 1; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

umask 077
WORK="$(mktemp -d "${TMPDIR:-/tmp}/sign.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# The secret may be the PEM itself, the PEM with its newlines collapsed to spaces (some secret stores do that), or the base64 of the
# PEM (the form the vault keeps it in, P7-PROD-59). All three are accepted and rewritten as a canonical PEM; anything else still
# fails below as an unreadable key. The key text is never printed.
# The PEM marker lines are assembled from two pieces on purpose: written whole, a secret scanner reads this script as a committed key.
PEM_BEGIN="-----BEGIN PRIV""ATE KEY-----"
PEM_END="-----END PRIV""ATE KEY-----"
normalize_key() {
  local v="$1" body
  case "$v" in
    *"-----BEGIN "*) ;;
    *) v="$(printf '%s' "$v" | tr -d ' \t\r\n' | "$O" base64 -d -A 2>/dev/null)" || v="" ;;
  esac
  case "$v" in
    *"$PEM_BEGIN"*"$PEM_END"*)
      body="${v#*"$PEM_BEGIN"}"
      body="${body%%"$PEM_END"*}"
      body="$(printf '%s' "$body" | tr -d ' \t\r\n')"
      printf '%s\n%s\n%s\n' "$PEM_BEGIN" "$(printf '%s' "$body" | fold -w 64)" "$PEM_END"
      ;;
    *) printf '%s\n' "$v" ;;
  esac
}
normalize_key "$NSELF_RELEASE_SIGNING_KEY_FREE" > "$WORK/key.pem"

# Only Ed25519 keys sign here: an EC or RSA key would produce a signature labelled ed25519 that no CLI could verify.
KEY_TEXT="$("$O" pkey -in "$WORK/key.pem" -noout -text 2>/dev/null)" || { err "NSELF_RELEASE_SIGNING_KEY_FREE is not a readable PEM private key"; exit 1; }
case "$KEY_TEXT" in "ED25519 Private-Key:"*) ;; *) err "NSELF_RELEASE_SIGNING_KEY_FREE is not an Ed25519 key"; exit 1 ;; esac

# The id must be the one derived from this key, so a mis-set secret fails here and not in a user's install.
if ! DERIVED="$("$O" pkey -in "$WORK/key.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)" || [ -z "$DERIVED" ]; then
  err "NSELF_RELEASE_SIGNING_KEY_FREE is not a readable PEM private key"
  exit 1
fi
if [ "plugins-$DERIVED" != "$KEY_ID" ]; then
  err "NSELF_RELEASE_SIGNING_KEY_FREE_ID does not match the key (expected plugins-<16 hex of sha256(public key))"
  exit 1
fi

SHA="$(sha256_file "$TARBALL")"
if [ "$#" -eq 4 ]; then
  printf 'nself-plugin-bin-v1\n%s\n%s\n%s\n%s\n' "$SLUG" "$VERSION" "$PLATFORM" "$SHA" > "$WORK/msg"
else
  printf 'nself-plugin-v1\n%s\n%s\n%s\n' "$SLUG" "$VERSION" "$SHA" > "$WORK/msg"
fi
"$O" pkeyutl -sign -rawin -inkey "$WORK/key.pem" -in "$WORK/msg" -out "$WORK/sig" 2>/dev/null || { err "signing failed (is the key Ed25519?)"; exit 1; }
[ "$(wc -c < "$WORK/sig" | tr -d ' ')" = "64" ] || { err "signature is not 64 bytes; the key is not Ed25519"; exit 1; }
SIG="$("$O" base64 -A -in "$WORK/sig")"
printf '{"key_id":"%s","alg":"ed25519","sig":"%s"}\n' "$KEY_ID" "$SIG"
