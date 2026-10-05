#!/usr/bin/env bash
# build.sh --plugins LIST [--push=true|false] [--out DIR] [--root DIR]: multi-arch image build per free plugin.
# LIST is comma separated slugs (the output of matrix.sh). Each plugin builds from its own directory with its
# own Dockerfile as linux/amd64 + linux/arm64 and is tagged nself/nself-<slug>:<version> and :latest.
# --provenance=false keeps attestation entries (unknown/unknown) out of the index.
# --push=false (default): exports an OCI layout instead; with --out DIR writes DIR/<slug>/image.tar, the extracted
#   layout in DIR/<slug>/layout and DIR/<slug>/index.json (the resolved image index, whose .manifests[] carry
#   .platform.architecture). Nothing leaves the machine.
# --push=true: pushes both tags and writes DIR/<slug>/metadata.json (holds containerimage.digest and version). Refuses unless
#   NSELF_PUBLISH_PLUGIN_IMAGES=true is in the environment (the first-publish gate; set by P7-PLUG-65 only).
# Needs a buildx builder that can build both platforms (docker-container driver + QEMU); BUILDX_BUILDER picks it.
# Exit: 0 ok, 1 a build failed or an index lacks a platform, 2 usage or gate.
set -eu
. "$(dirname "$0")/lib.sh"
img_need jq; img_need docker
root=$IMG_ROOT; plugins=""; push=false; out=""
while [ $# -gt 0 ]; do
  case $1 in
    --plugins) plugins=$2; shift 2 ;;
    --push=true) push=true; shift ;;
    --push=false) push=false; shift ;;
    --out) out=$2; shift 2 ;;
    --root) root=$2; shift 2 ;;
    *) echo "usage: build.sh --plugins LIST [--push=true|false] [--out DIR] [--root DIR]" >&2; exit 2 ;;
  esac
done
[ -n "$plugins" ] || { echo "build: --plugins LIST is required" >&2; exit 2; }
if [ "$push" = true ] && [ "${NSELF_PUBLISH_PLUGIN_IMAGES:-}" != true ]; then
  echo "skipped push: NSELF_PUBLISH_PLUGIN_IMAGES != true" >&2; exit 2
fi
[ -n "$out" ] || { out=$(img_tmp); cleanup=1; }
mkdir -p "$out"
revision=$(git -C "$root" rev-parse HEAD 2>/dev/null || echo unknown)

for slug in $(printf '%s' "$plugins" | tr ',' ' '); do
  img_slug_ok "$slug" || { echo "build: bad slug '$slug'" >&2; exit 2; }
  dir=$root/free/$slug
  [ -f "$dir/Dockerfile" ] && [ -f "$dir/plugin.json" ] || { echo "build: $dir has no Dockerfile or plugin.json" >&2; exit 1; }
  version=$(jq -r '.version // empty' "$dir/plugin.json")
  [ -n "$version" ] || { echo "build: $dir/plugin.json has no version" >&2; exit 1; }
  repo=nself/nself-$slug
  mkdir -p "$out/$slug"
  set -- --platform "$IMG_PLATFORMS" --provenance=false --sbom=false \
    --label "org.opencontainers.image.source=$IMG_SOURCE_URL" \
    --label "org.opencontainers.image.revision=$revision" \
    --label "org.opencontainers.image.version=$version" \
    -t "$repo:$version" -f "$dir/Dockerfile"
  # A pre-release version (1.2.0-rc1) never moves :latest.
  case $version in *-*) echo "build: $version is a pre-release; :latest not tagged" ;; *) set -- "$@" -t "$repo:latest" ;; esac
  if [ "$push" = true ]; then
    echo "build: $repo:$version pushing"
    docker buildx build "$@" --push --metadata-file "$out/$slug/metadata.json" "$dir"
    jq --arg v "$version" '. + {version: $v}' "$out/$slug/metadata.json" > "$out/$slug/metadata.tmp" && mv "$out/$slug/metadata.tmp" "$out/$slug/metadata.json"
    continue
  fi
  echo "build: $repo:$version (no push)"
  docker buildx build "$@" --output "type=oci,dest=$out/$slug/image.tar" "$dir"
  rm -rf "$out/$slug/layout"; mkdir -p "$out/$slug/layout"
  tar -xf "$out/$slug/image.tar" -C "$out/$slug/layout"
  top=$(jq -r '.manifests[0].digest | sub("^sha256:"; "")' "$out/$slug/layout/index.json")
  cp "$out/$slug/layout/blobs/sha256/$top" "$out/$slug/index.json"
  jq -e '[.manifests[].platform.architecture] | sort == ["amd64","arm64"]' "$out/$slug/index.json" >/dev/null \
    || { echo "build: $slug index does not carry exactly amd64 and arm64" >&2; exit 1; }
  echo "build: $slug index ok ($(jq -r '[.manifests[].platform.architecture] | join(",")' "$out/$slug/index.json"))"
done
[ -z "${cleanup:-}" ] || rm -rf "$out"
