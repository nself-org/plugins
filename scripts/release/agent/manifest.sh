#!/usr/bin/env bash
# manifest.sh --in DIR [--rehearsal] [--protocol-min N] [--protocol-max N]
#
# Purpose: write and sign one agent-release/v1 manifest per nself-ci-agent binary in DIR (hq NODE D11, contract
#   plugins.release-signature v1 with the agent-purpose key). The signed message is the plain, domain-separated string
#       nself-agent-v1\n<target>\n<version>\n<sha256 hex of the binary>\n<protocol_min>-<protocol_max>\n
#   (every field followed by a newline, the last one too; no canonical JSON), signed with raw Ed25519 through the same
#   `openssl pkeyutl -sign -rawin` primitive and the same base64 encoding as scripts/release/sign.sh, so cli sdk/go/signing verifies both.
#   free/ci/internal/agent/verify builds the same bytes (verify.SigningInput); a golden test keeps the two in step.
#
# Inputs:  --in DIR        output of build.sh (nself-ci-agent-<os>-<arch> files and VERSION).
#          --rehearsal     sign with a THROWAWAY key generated here and write its public half to DIR/rehearsal-test.pub. The
#                          environment key is never read. No other mode generates a key.
#          --protocol-min / --protocol-max   the runner protocol range this agent speaks (default 1 and 1).
#   Environment (never argv), outside rehearsal:
#          NSELF_AGENT_SIGNING_KEY      PKCS8 PEM of the Ed25519 agent key (raw PEM, PEM with collapsed newlines, or base64 of the PEM).
#          NSELF_AGENT_SIGNING_KEY_ID   agent-<first 16 hex of sha256(raw 32-byte public key)>; the id is derived, never chosen.
#          OPENSSL                      optional openssl binary; must be OpenSSL 3 or newer (LibreSSL cannot sign Ed25519 -rawin).
# Output:  DIR/nself-ci-agent-<os>-<arch>.manifest.json, one line each:
#   {"schema":"agent-release/v1","version":"1.5.0","target":"linux-amd64","sha256":"<hex>","protocol_min":1,"protocol_max":1,
#    "key_id":"agent-<16 hex>","alg":"ed25519","sig":"<base64>"}
# Exit 0 ok, 1 failure, 2 usage. The key lives in a 0600 file under a 0700 temp dir that is removed on exit; it is never printed,
# logged or written under DIR.

set -euo pipefail
set +x

err() { printf "[agent-manifest] ERROR: %s\n" "$*" >&2; }

IN=""
REHEARSAL=false
PMIN=1
PMAX=1
while [ "$#" -gt 0 ]; do
  case "$1" in
    --in) [ "$#" -ge 2 ] || { err "--in needs a value"; exit 2; }; IN="$2"; shift 2 ;;
    --rehearsal) REHEARSAL=true; shift ;;
    --protocol-min) [ "$#" -ge 2 ] || { err "--protocol-min needs a value"; exit 2; }; PMIN="$2"; shift 2 ;;
    --protocol-max) [ "$#" -ge 2 ] || { err "--protocol-max needs a value"; exit 2; }; PMAX="$2"; shift 2 ;;
    *) err "unknown argument: $1"; printf "usage: %s --in DIR [--rehearsal] [--protocol-min N] [--protocol-max N]\n" "$0" >&2; exit 2 ;;
  esac
done
if [ -z "$IN" ]; then
  printf "usage: %s --in DIR [--rehearsal] [--protocol-min N] [--protocol-max N]\n" "$0" >&2
  exit 2
fi
[[ "$PMIN" =~ ^[1-9][0-9]{0,5}$ ]] && [[ "$PMAX" =~ ^[1-9][0-9]{0,5}$ ]] && [ "$PMIN" -le "$PMAX" ] || { err "protocol range $PMIN-$PMAX is not valid"; exit 2; }
[ -d "$IN" ] || { err "input directory not found: $IN"; exit 1; }
IN="$(cd "$IN" && pwd)"
[ -f "$IN/VERSION" ] || { err "$IN/VERSION not found (run build.sh first)"; exit 1; }
VERSION="$(tr -d '\n' < "$IN/VERSION")"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { err "VERSION '$VERSION' is not a plain version"; exit 1; }

O="${OPENSSL:-openssl}"
command -v "$O" >/dev/null 2>&1 || { err "openssl not found (set OPENSSL to an OpenSSL 3 binary)"; exit 1; }
"$O" version 2>/dev/null | grep -q '^OpenSSL ' || { err "$O is not OpenSSL (LibreSSL cannot sign Ed25519 -rawin); set OPENSSL"; exit 1; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

umask 077
WORK="$(mktemp -d "${TMPDIR:-/tmp}/agent-manifest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# The PEM marker lines are assembled from two pieces on purpose: written whole, a secret scanner reads this script as a committed key.
PEM_BEGIN="-----BEGIN PRIV""ATE KEY-----"
PEM_END="-----END PRIV""ATE KEY-----"
# Same three accepted forms as sign.sh (raw PEM, PEM with newlines collapsed to spaces, base64 of the PEM).
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

if [ "$REHEARSAL" = "true" ]; then
  [ ! -e "$IN/rehearsal-test.pub" ] || { err "$IN/rehearsal-test.pub already exists"; exit 1; }
  "$O" genpkey -algorithm ed25519 -out "$WORK/key.pem" 2>/dev/null
  "$O" pkey -in "$WORK/key.pem" -pubout -out "$IN/rehearsal-test.pub" 2>/dev/null
  chmod 644 "$IN/rehearsal-test.pub"
else
  [ -n "${NSELF_AGENT_SIGNING_KEY:-}" ] || { err "NSELF_AGENT_SIGNING_KEY is not set"; exit 1; }
  normalize_key "$NSELF_AGENT_SIGNING_KEY" > "$WORK/key.pem"
fi

# Only Ed25519 keys sign here: an EC or RSA key would produce a signature labelled ed25519 that no CLI could verify.
KEY_TEXT="$("$O" pkey -in "$WORK/key.pem" -noout -text 2>/dev/null)" || { err "the signing key is not a readable PEM private key"; exit 1; }
case "$KEY_TEXT" in "ED25519 Private-Key:"*) ;; *) err "the signing key is not an Ed25519 key"; exit 1 ;; esac
if ! DERIVED="$("$O" pkey -in "$WORK/key.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)" || [ -z "$DERIVED" ]; then
  err "the signing key is not a readable PEM private key"
  exit 1
fi
KEY_ID="agent-${DERIVED}"
if [ "$REHEARSAL" != "true" ]; then
  [[ "${NSELF_AGENT_SIGNING_KEY_ID:-}" =~ ^agent-[0-9a-f]{16}$ ]] || { err "NSELF_AGENT_SIGNING_KEY_ID must look like agent-<16 hex>"; exit 1; }
  [ "$NSELF_AGENT_SIGNING_KEY_ID" = "$KEY_ID" ] || { err "NSELF_AGENT_SIGNING_KEY_ID does not match the key (expected agent-<16 hex of sha256(public key))"; exit 1; }
fi

count=0
for bin in "$IN"/nself-ci-agent-*-*; do
  [ -f "$bin" ] || continue
  name="$(basename "$bin")"
  case "$name" in *.manifest.json|*.pub) continue ;; esac
  target="${name#nself-ci-agent-}"
  [[ "$target" =~ ^(linux|darwin)-(amd64|arm64)$ ]] || { err "unexpected file name $name"; exit 1; }
  sha="$(sha256_file "$bin")"
  printf 'nself-agent-v1\n%s\n%s\n%s\n%s-%s\n' "$target" "$VERSION" "$sha" "$PMIN" "$PMAX" > "$WORK/msg"
  "$O" pkeyutl -sign -rawin -inkey "$WORK/key.pem" -in "$WORK/msg" -out "$WORK/sig" 2>/dev/null || { err "signing failed (is the key Ed25519?)"; exit 1; }
  [ "$(wc -c < "$WORK/sig" | tr -d ' ')" = "64" ] || { err "signature is not 64 bytes; the key is not Ed25519"; exit 1; }
  sig="$("$O" base64 -A -in "$WORK/sig")"
  printf '{"schema":"agent-release/v1","version":"%s","target":"%s","sha256":"%s","protocol_min":%s,"protocol_max":%s,"key_id":"%s","alg":"ed25519","sig":"%s"}\n' \
    "$VERSION" "$target" "$sha" "$PMIN" "$PMAX" "$KEY_ID" "$sig" > "$IN/${name}.manifest.json"
  chmod 644 "$IN/${name}.manifest.json"
  count=$((count + 1))
done
[ "$count" -gt 0 ] || { err "no nself-ci-agent-<os>-<arch> files in $IN"; exit 1; }
printf "[agent-manifest] signed %s manifests, key %s\n" "$count" "$KEY_ID"
