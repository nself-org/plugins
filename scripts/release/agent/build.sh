#!/usr/bin/env bash
# build.sh --version V --out DIR
#
# Purpose: build nself-ci-agent for the three supported targets (linux/amd64, linux/arm64, darwin/arm64) with the reproducible recipe of
#   hq NODE D11: CGO_ENABLED=0, -trimpath, -buildvcs=false, an empty build id, vendored modules. Two builds of the same tree and version
#   give byte-identical files; the release workflow and the rehearsal both call this script, so they cannot drift.
#
# Inputs:  --version V   v1.5.0 or 1.5.0, optionally with a pre-release suffix (v0.0.0-rehearsal). The leading v is dropped.
#          --out DIR     created; must be empty or absent.
#          SOURCE_DATE_EPOCH   optional; default is the commit time of HEAD (the tag commit in the workflow).
# Output in DIR:
#   nself-ci-agent-<os>-<arch>   one static binary per target (mode 0755, mtime = SOURCE_DATE_EPOCH)
#   VERSION                      the version without the leading v, one line (manifest.sh reads it)
#   SHA256SUMS                   "<sha256>  <file>" per binary, sorted
# The main.Version the binary reports is the x.y.z core of V (the coordinator compares it with min_agent_version, so a suffix cannot
# be in it); manifest.sh signs the full V.
# Exit 0 ok, 1 failure, 2 usage. No key, no network and no secret is read here.

set -euo pipefail
set +x

err() { printf "[agent-build] ERROR: %s\n" "$*" >&2; }

VERSION=""
OUT=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || { err "--version needs a value"; exit 2; }; VERSION="$2"; shift 2 ;;
    --out) [ "$#" -ge 2 ] || { err "--out needs a value"; exit 2; }; OUT="$2"; shift 2 ;;
    *) err "unknown argument: $1"; printf "usage: %s --version V --out DIR\n" "$0" >&2; exit 2 ;;
  esac
done
if [ -z "$VERSION" ] || [ -z "$OUT" ]; then
  printf "usage: %s --version V --out DIR\n" "$0" >&2
  exit 2
fi
VERSION="${VERSION#v}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { err "version '$VERSION' must look like 1.2.3 or 1.2.3-rc.1"; exit 2; }
CORE="${VERSION%%-*}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
CI_DIR="${REPO_ROOT}/free/ci"
[ -d "$CI_DIR/cmd/agent" ] || { err "free/ci/cmd/agent not found under $REPO_ROOT"; exit 1; }
command -v go >/dev/null 2>&1 || { err "go not found"; exit 1; }

mkdir -p "$OUT"
if [ -n "$(ls -A "$OUT")" ]; then
  err "output directory is not empty: $OUT"
  exit 1
fi
OUT="$(cd "$OUT" && pwd)"

if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
  SOURCE_DATE_EPOCH="$(git -C "$REPO_ROOT" log -1 --format=%ct 2>/dev/null || true)"
fi
[[ "${SOURCE_DATE_EPOCH:-}" =~ ^[0-9]+$ ]] || { err "SOURCE_DATE_EPOCH is not set and HEAD has no commit time"; exit 1; }
export SOURCE_DATE_EPOCH

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# Same mtime on GNU and BSD touch, so a copied directory is also stable.
set_mtime() {
  if touch -d "@${SOURCE_DATE_EPOCH}" "$1" 2>/dev/null; then return 0; fi
  touch -t "$(date -u -r "${SOURCE_DATE_EPOCH}" +%Y%m%d%H%M.%S)" "$1"
}

TARGETS="linux/amd64 linux/arm64 darwin/arm64"
: > "$OUT/SHA256SUMS.tmp"
built=0
for t in $TARGETS; do
  goos="${t%/*}"
  goarch="${t#*/}"
  name="nself-ci-agent-${goos}-${goarch}"
  # Go's own environment must not change the recipe: no GOFLAGS, no workspace, no VCS stamp.
  (
    cd "$CI_DIR"
    env -u GOFLAGS GOWORK=off CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -mod=vendor -trimpath -buildvcs=false \
        -ldflags "-s -w -buildid= -X main.Version=${CORE}" \
        -o "${OUT}/${name}" ./cmd/agent
  ) || { err "build failed for ${goos}/${goarch}"; exit 1; }
  chmod 755 "${OUT}/${name}"
  set_mtime "${OUT}/${name}"
  printf '%s  %s\n' "$(sha256_file "${OUT}/${name}")" "$name" >> "$OUT/SHA256SUMS.tmp"
  built=$((built + 1))
done
LC_ALL=C sort -k2 "$OUT/SHA256SUMS.tmp" > "$OUT/SHA256SUMS"
rm -f "$OUT/SHA256SUMS.tmp"
printf '%s\n' "$VERSION" > "$OUT/VERSION"
set_mtime "$OUT/SHA256SUMS"
set_mtime "$OUT/VERSION"
printf "[agent-build] built %s targets, version %s, SOURCE_DATE_EPOCH %s\n" "$built" "$VERSION" "$SOURCE_DATE_EPOCH"
