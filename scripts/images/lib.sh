#!/usr/bin/env bash
# lib.sh: shared helpers for scripts/images/* (P7-PLUG-61). Source it; it defines functions only.
# Needs bash 3.2+ and jq.
IMG_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
IMG_DIR=$IMG_ROOT/scripts/images
IMG_SOURCE_URL=https://github.com/nself-org/plugins
IMG_PLATFORMS="linux/amd64,linux/arm64"

img_need() { command -v "$1" >/dev/null 2>&1 || { echo "images: $1 is required" >&2; exit 2; }; }
img_slug_ok() { printf '%s' "$1" | grep -Eq '^[a-z][a-z0-9-]*$'; }
# img_tmp prints a fresh scratch directory (caller removes it).
img_tmp() { mktemp -d "${TMPDIR:-/tmp}/nself-images.XXXXXX"; }
