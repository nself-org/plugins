#!/usr/bin/env bash
# rehearse.sh --tag TAG --no-upload --out DIR
#
# Purpose: dry-run the signing half of a plugins release without a release, a tag, a network or the production key.
#   It builds the source tarball of every free plugin with the shared deterministic recipe (scripts/deterministic-tar.sh, the
#   same recipe .github/workflows/release-tarballs.yml uses), signs each with a THROWAWAY Ed25519 key generated here
#   (scripts/release/sign.sh, the same signer the workflow uses), and writes the release data the verifier reads.
#   Two runs of the same tag give byte-identical tarballs; the signatures differ because the key is new each run.
#
# Output in DIR (created; must be empty or absent):
#   <slug>-<version>.tar.gz   source tarballs, version = TAG without the leading v
#   <slug>-<version>-<platform>.tar.gz
#                             fixture binary tarballs for the FIRST slug only (contract v1.1): one per platform, each a deterministic
#                             tarball of a stub file nself-<slug> (never a real build, no toolchain, no network), signed under the
#                             platform domain with the same throwaway key
#   releases.json             release-data shape: {"releases": {"<slug>": {version, sha256, release_tag, release_signature}}}; the
#                             first slug's row also carries platform_checksums and platform_signatures
#   rehearsal-test.pub        PEM SPKI public half of the throwaway key. It is the only key these signatures verify against:
#                             scripts/release/verify-signatures.sh --pub DIR/rehearsal-test.pub DIR
#
# The throwaway private key lives in a 0700 temp dir that is removed on exit; it is never written under DIR or the repo.
# This script never reads NSELF_RELEASE_SIGNING_KEY_* from the caller (they are unset for the signer) and never uploads:
# --no-upload is required so a future upload mode cannot be reached by accident. The real run is the workflow, P7-PLUG-69.
#
# Requires GNU tar (see deterministic-tar.sh), jq, OpenSSL 3 (set OPENSSL), run from any directory.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

err() { printf "[rehearse] ERROR: %s\n" "$*" >&2; }
log() { printf "[rehearse] %s\n" "$*"; }

TAG=""
OUT=""
NO_UPLOAD=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --tag) TAG="${2:-}"; shift 2 || true ;;
    --out) OUT="${2:-}"; shift 2 || true ;;
    --no-upload) NO_UPLOAD=true; shift ;;
    *) err "unknown argument: $1"; printf "usage: %s --tag TAG --no-upload --out DIR\n" "$0" >&2; exit 2 ;;
  esac
done
if [ -z "$TAG" ] || [ -z "$OUT" ] || [ "$NO_UPLOAD" != "true" ]; then
  printf "usage: %s --tag TAG --no-upload --out DIR\n" "$0" >&2
  exit 2
fi
[[ "$TAG" =~ ^v[0-9][0-9A-Za-z.+-]*$ ]] || { err "tag '$TAG' must look like v1.2.3 or v0.0.0-rehearsal"; exit 2; }
VERSION="${TAG#v}"

O="${OPENSSL:-openssl}"
command -v jq >/dev/null 2>&1 || { err "jq not found"; exit 1; }
command -v "$O" >/dev/null 2>&1 || { err "openssl not found (set OPENSSL to an OpenSSL 3 binary)"; exit 1; }
"$O" version 2>/dev/null | grep -q '^OpenSSL ' || { err "$O is not OpenSSL (LibreSSL cannot sign Ed25519 -rawin); set OPENSSL"; exit 1; }

mkdir -p "$OUT"
if [ -n "$(ls -A "$OUT")" ]; then
  err "output directory is not empty: $OUT"
  exit 1
fi
OUT="$(cd "$OUT" && pwd)"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

umask 077
KEYDIR="$(mktemp -d "${TMPDIR:-/tmp}/rehearse-key.XXXXXX")"
trap 'rm -rf "$KEYDIR"' EXIT
"$O" genpkey -algorithm ed25519 -out "$KEYDIR/key.pem" 2>/dev/null
"$O" pkey -in "$KEYDIR/key.pem" -pubout -out "$OUT/rehearsal-test.pub" 2>/dev/null
chmod 644 "$OUT/rehearsal-test.pub"
KEY_ID="plugins-$("$O" pkey -in "$KEYDIR/key.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)"
umask 022

cd "$REPO_ROOT"
ROWS="$KEYDIR/rows.ndjson"
: > "$ROWS"
count=0
while IFS= read -r dir; do
  [ -f "$dir/plugin.json" ] || continue
  slug="$(basename "$dir")"
  tarball="${OUT}/${slug}-${VERSION}.tar.gz"
  bash scripts/deterministic-tar.sh "$tarball" "$dir"
  sha="$(sha256_file "$tarball")"
  # The throwaway key is handed to the signer through its environment only, for this one call.
  sig="$(NSELF_RELEASE_SIGNING_KEY_FREE="$(cat "$KEYDIR/key.pem")" NSELF_RELEASE_SIGNING_KEY_FREE_ID="$KEY_ID" \
    OPENSSL="$O" bash scripts/release/sign.sh "$slug" "$VERSION" "$tarball")"
  plat='{}'
  psig='{}'
  if [ "$count" -eq 0 ]; then
    stub="$KEYDIR/stub"
    mkdir -p "$stub"
    for platform in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64; do
      printf 'stub binary for %s %s %s\n' "$slug" "$VERSION" "$platform" > "$stub/nself-${slug}"
      chmod 755 "$stub/nself-${slug}"
      ptar="${OUT}/${slug}-${VERSION}-${platform}.tar.gz"
      bash scripts/deterministic-tar.sh "$ptar" -C "$stub" "nself-${slug}"
      psha="$(sha256_file "$ptar")"
      psignature="$(NSELF_RELEASE_SIGNING_KEY_FREE="$(cat "$KEYDIR/key.pem")" NSELF_RELEASE_SIGNING_KEY_FREE_ID="$KEY_ID" \
        OPENSSL="$O" bash scripts/release/sign.sh "$slug" "$VERSION" "$ptar" "$platform")"
      plat="$(jq -c --arg p "$platform" --arg sha "$psha" '.[$p] = $sha' <<<"$plat")"
      psig="$(jq -c --arg p "$platform" --argjson sig "$psignature" '.[$p] = $sig' <<<"$psig")"
    done
  fi
  jq -nc --arg s "$slug" --arg v "$VERSION" --arg t "$TAG" --arg sha "$sha" --argjson sig "$sig" --argjson plat "$plat" --argjson psig "$psig" \
    '{($s): ({version: $v, sha256: $sha, release_tag: $t, release_signature: $sig}
      + (if ($plat | length) > 0 then {platform_checksums: $plat, platform_signatures: $psig} else {} end))}' >> "$ROWS"
  count=$((count + 1))
done < <(find free -maxdepth 1 -mindepth 1 -type d | sort)

[ "$count" -gt 0 ] || { err "no free plugin with a plugin.json found under $REPO_ROOT/free"; exit 1; }
jq -sS '{schema_version: 1, releases: (add)}' "$ROWS" > "$OUT/releases.json"
log "built and signed ${count} source tarball(s) (and the fixture binary tarballs of the first slug) for ${TAG} into ${OUT} (throwaway key ${KEY_ID}; public half: rehearsal-test.pub)"
