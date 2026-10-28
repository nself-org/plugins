#!/usr/bin/env bash
# Purpose: sign the published source/platform set and verify it against pinned release keys.
# Inputs: TAG WORKSPACE [KEYS_DIR]; workspace contains published/, free/ and expected/built-sha256.txt.
#   Signing credentials and OPENSSL are environment-only, as for sign.sh.
# Output: WORKSPACE/signatures.json, only after every built check and signature verification passes.
# Constraints: no network or uploads; KEYS_DIR defaults to this checkout's committed keys (fixtures may supply throwaway keys).
set -euo pipefail
set +x
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[ "$#" -ge 2 ] && [ "$#" -le 3 ] || { echo "usage: $0 TAG WORKSPACE [KEYS_DIR]" >&2; exit 2; }
VERSION="${1#v}"
KEYS="$(cd "${3:-$HERE/keys}" && pwd)"
cd "$2"
# A refused rerun must not expose signatures from an earlier successful run.
rm -f signatures.json
if [ -z "${NSELF_RELEASE_SIGNING_KEY_FREE:-}" ] || [ -z "${NSELF_RELEASE_SIGNING_KEY_FREE_ID:-}" ]; then
  echo "ERROR: the release-signing Environment has no free-tier signing key and key id"
  exit 1
fi
WORK="$(mktemp -d "${TMPDIR:-/tmp}/sign-release.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
sha256_file() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
: > "$WORK/signatures.ndjson"
count=0
for f in published/*-"${VERSION}".tar.gz; do
  [ -e "$f" ] || continue
  slug="$(basename "$f" "-${VERSION}.tar.gz")"
  # A source tarball must match the manifest version for this tag.
  if [ "$(jq -r '.version // ""' "free/${slug}/plugin.json" 2>/dev/null)" != "$VERSION" ]; then
    echo "SKIP ${slug}: manifest version is not ${VERSION}"
    continue
  fi
  sig="$(bash "$HERE/sign.sh" "$slug" "$VERSION" "$f")"
  sha="$(sha256_file "$f")"
  jq -nc --arg s "$slug" --arg sha "$sha" --argjson sig "$sig" '{($s): {sha256: $sha, release_signature: $sig}}' >> "$WORK/signatures.ndjson"
  count=$((count + 1))
done
if [ "$count" -eq 0 ]; then
  echo "ERROR: no source tarball to sign in published/"
  exit 1
fi
jq -sS 'add' "$WORK/signatures.ndjson" > "$WORK/signatures.json"
echo "Signed ${count} source tarball(s)"

# Only platforms of signed sources are eligible, and every one must have been built by this run.
platforms="darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64"
bin_stage="$WORK/platforms"
mkdir "$bin_stage"
for slug in $(jq -r 'keys[]' "$WORK/signatures.json"); do
  for platform in $platforms; do
    f="published/${slug}-${VERSION}-${platform}.tar.gz"
    [ -e "$f" ] || continue
    ln -s "${PWD}/${f}" "$bin_stage/"
  done
done
if [ -z "$(ls -A "$bin_stage")" ]; then
  echo "No platform tarball for the signed plugins; nothing to sign beyond the source tarballs"
elif ! bash "$HERE/platform-built-check.sh" expected/built-sha256.txt "$bin_stage"; then
  echo "ERROR: a platform tarball on the Release was not built by this run; nothing is signed. See Releasing.md."
  exit 1
fi
: > "$WORK/platform-signatures.ndjson"
pcount=0
for slug in $(jq -r 'keys[]' "$WORK/signatures.json"); do
  for platform in $platforms; do
    f="published/${slug}-${VERSION}-${platform}.tar.gz"
    [ -e "$f" ] || continue
    psig="$(bash "$HERE/sign.sh" "$slug" "$VERSION" "$f" "$platform")"
    psha="$(sha256_file "$f")"
    jq -nc --arg s "$slug" --arg p "$platform" --arg sha "$psha" --argjson sig "$psig" \
      '{($s): {platform_signatures: {($p): ($sig + {sha256: $sha})}}}' >> "$WORK/platform-signatures.ndjson"
    pcount=$((pcount + 1))
  done
done
jq -sS 'reduce .[1:][] as $r (.[0]; . * $r)' "$WORK/signatures.json" "$WORK/platform-signatures.ndjson" > "$WORK/signatures.merged.json"
mv "$WORK/signatures.merged.json" "$WORK/signatures.json"
echo "Signed ${pcount} platform tarball(s)"

# Mirror release-data: checksums carry sha256; signature objects do not.
stage="$WORK/verify"
mkdir "$stage"
jq --arg v "$VERSION" 'map_values(. + {version: $v}
    + (if has("platform_signatures") then {platform_checksums: (.platform_signatures | map_values(.sha256)),
                                            platform_signatures: (.platform_signatures | map_values(del(.sha256)))} else {} end))
  | {releases: .}' "$WORK/signatures.json" > "$stage/releases.json"
for slug in $(jq -r 'keys[]' "$WORK/signatures.json"); do
  ln -s "${PWD}/published/${slug}-${VERSION}.tar.gz" "$stage/"
done
for f in "$bin_stage"/*.tar.gz; do
  [ -e "$f" ] || continue
  ln -s "$(readlink "$f")" "$stage/"
done
pinned=0
ok=0
for k in "$KEYS"/plugins-release-free-*.pub; do
  [ -e "$k" ] || continue
  pinned=1
  if bash "$HERE/verify-signatures.sh" --pub "$k" "$stage" >/dev/null 2>&1; then ok=1; fi
done
if [ "$pinned" -ne 1 ]; then
  echo "ERROR: scripts/release/keys/ holds no release public key; commit it before a real tag run"
  exit 1
fi
if [ "$ok" -ne 1 ]; then
  echo "ERROR: the signatures do not verify against any committed release key in scripts/release/keys/; the signing secret is not a pinned key"
  exit 1
fi
mv "$WORK/signatures.json" signatures.json
