#!/usr/bin/env bash
# Sign staged published assets; an optional public-key directory supports offline fixtures.
set -euo pipefail
set +x
VERSION="${TAG#v}"
HERE="$(cd "$(dirname "$0")" && pwd)"
KEY_DIR="${1:-$HERE/keys}"
if [ -z "${NSELF_RELEASE_SIGNING_KEY_FREE:-}" ] || [ -z "${NSELF_RELEASE_SIGNING_KEY_FREE_ID:-}" ]; then
  echo "ERROR: the release-signing Environment has no free-tier signing key and key id (P7-PROD-59 creates them)"
  exit 1
fi
: > signatures.ndjson
count=0
for f in published/*-"${VERSION}".tar.gz; do
  [ -e "$f" ] || continue
  slug="$(basename "$f" "-${VERSION}.tar.gz")"
  # A tarball named for this tag whose manifest declares another version is not a release of that plugin: leave it unsigned.
  if [ "$(jq -r '.version // ""' "free/${slug}/plugin.json" 2>/dev/null)" != "$VERSION" ]; then
    echo "SKIP ${slug}: manifest version is not ${VERSION}"
    continue
  fi
  sig="$(bash "$HERE/sign.sh" "$slug" "$VERSION" "$f")"
  sha="$(sha256sum "$f" | cut -d' ' -f1)"
  jq -nc --arg s "$slug" --arg sha "$sha" --argjson sig "$sig" '{($s): {sha256: $sha, release_signature: $sig}}' >> signatures.ndjson
  count=$((count + 1))
done
if [ "$count" -eq 0 ]; then
  echo "ERROR: no source tarball to sign in published/"
  exit 1
fi
jq -sS 'add' signatures.ndjson > signatures.json
rm -f signatures.ndjson
echo "Signed ${count} source tarball(s)"

# Platform tarballs of the signed plugins only (a plugin left unsigned above gets none). Every one must be a tarball this
# run built: the Release never takes a clobbering upload, so an asset put there by anyone else during the approval wait
# is already published, and signing it would hand it the official platform signature.
platforms="darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64"
bin_stage="$(mktemp -d)"
stage=""
trap 'rm -rf "$bin_stage"; if [ -n "$stage" ]; then rm -rf "$stage"; fi' EXIT
for slug in $(jq -r 'keys[]' signatures.json); do
  for platform in $platforms; do
    f="published/${slug}-${VERSION}-${platform}.tar.gz"
    [ -e "$f" ] || continue
    ln -s "${PWD}/${f}" "$bin_stage/"
  done
done
if [ -z "$(ls -A "$bin_stage")" ]; then
  echo "No platform tarball for the signed plugins; nothing to sign beyond the source tarballs"
elif ! bash "$HERE/platform-built-check.sh" expected/built-sha256.txt "$bin_stage"; then
  echo "ERROR: a platform tarball on the Release was not built by this run; nothing is signed. Delete that platform tarball AND its .sha256 from the Release, then re-run the release (Releasing.md)."
  exit 1
fi
: > platform-signatures.ndjson
pcount=0
for slug in $(jq -r 'keys[]' signatures.json); do
  for platform in $platforms; do
    f="published/${slug}-${VERSION}-${platform}.tar.gz"
    [ -e "$f" ] || continue
    psig="$(bash "$HERE/sign.sh" "$slug" "$VERSION" "$f" "$platform")"
    psha="$(sha256sum "$f" | cut -d' ' -f1)"
    jq -nc --arg s "$slug" --arg p "$platform" --arg sha "$psha" --argjson sig "$psig" \
      '{($s): {platform_signatures: {($p): ($sig + {sha256: $sha})}}}' >> platform-signatures.ndjson
    pcount=$((pcount + 1))
  done
done
jq -sS 'reduce .[1:][] as $r (.[0]; . * $r)' signatures.json platform-signatures.ndjson > signatures.merged.json
mv signatures.merged.json signatures.json
rm -f platform-signatures.ndjson
echo "Signed ${pcount} platform tarball(s)"

# Once a release public key is committed (P7-PLUG-69 commits the first), only a committed key may have signed.
# The stage mirrors releases.json: platform_checksums from the covered sha256, platform_signatures without it.
stage="$(mktemp -d)"
jq --arg v "$VERSION" 'map_values(. + {version: $v}
    + (if has("platform_signatures") then {platform_checksums: (.platform_signatures | map_values(.sha256)),
                                            platform_signatures: (.platform_signatures | map_values(del(.sha256)))} else {} end))
  | {releases: .}' signatures.json > "$stage/releases.json"
for slug in $(jq -r 'keys[]' signatures.json); do
  ln -s "${PWD}/published/${slug}-${VERSION}.tar.gz" "$stage/"
done
for f in "$bin_stage"/*.tar.gz; do
  [ -e "$f" ] || continue
  ln -s "$(readlink "$f")" "$stage/"
done
pinned=0
ok=0
for k in "$KEY_DIR"/plugins-release-free-*.pub; do
  [ -e "$k" ] || continue
  pinned=1
  if bash "$HERE/verify-signatures.sh" --pub "$k" "$stage" >/dev/null 2>&1; then ok=1; fi
done
if [ "$pinned" -ne 1 ]; then
  echo "ERROR: scripts/release/keys/ holds no release public key; commit it (P7-PLUG-69) before a real tag run, so the signatures can be checked against it"
  exit 1
fi
if [ "$ok" -ne 1 ]; then
  echo "ERROR: the signatures do not verify against any committed release key in scripts/release/keys/; the signing secret is not a pinned key"
  exit 1
fi
