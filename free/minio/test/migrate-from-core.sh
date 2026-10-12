#!/usr/bin/env bash
# Purpose: prove the minio plugin keeps core MinIO data under v1.5. A v1.4-mode project with MINIO_ENABLED=true
#          stores an object; the same project in v1.5 mode (NSELF_V15=1) refuses to build (E138), installs the
#          plugin from a local archive, and serves the same object from the same <project>_minio_data volume
#          on the pinned image.
# Usage:   migrate-from-core.sh
# Env:     NSELF   path of the nself checkout: either the cli directory itself or its parent holding cli/
#                  (default: ../../../../cli relative to the plugins repo, then ~/Sites/nself).
#          TMPDIR  optional scratch parent (default /tmp).
# Needs:   Go, openssl, tar, awk, a Docker daemon that can run a privileged docker:dind sibling (Linux containers).
# Outputs: PASS/FAIL lines; the last line is PASS on success.
# Exit:    0 PASS, 1 FAIL, 2 usage, 3 missing precondition.
# Constraints: bash 3.2 compatible. The CLI is cross-built static for the Docker daemon's architecture and copied into
#          a unique sibling dind container, so no host bind paths are needed. Credentials are random, synthetic,
#          travel only in 0600 files and never on argv. The sibling and the scratch dir are removed on every exit.
#          The in-sibling flow lives in lib-sibling.sh.
set -euo pipefail

[ "$#" = 0 ] || { echo "usage: migrate-from-core.sh (no arguments; see the header for env)" >&2; exit 2; }
HERE="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_DIR="$(cd "$HERE/.." && pwd)"
REPO_ROOT="$(cd "$PLUGIN_DIR/../.." && pwd)"
fail() { echo "FAIL: $1"; exit 1; }
need() { echo "FAIL: $1"; exit 3; }

# NSELF may name the cli directory or the nself root that holds cli/.
CLI="${NSELF:-}"
if [ -z "$CLI" ]; then
  for guess in "$REPO_ROOT/../cli" "$HOME/Sites/nself/cli"; do
    [ -f "$guess/go.mod" ] && { CLI="$guess"; break; }
  done
fi
[ -n "$CLI" ] || need "NSELF is not set and no cli checkout was found"
[ -f "$CLI/internal/compose/images.lock.json" ] || CLI="$CLI/cli"
[ -f "$CLI/internal/compose/images.lock.json" ] || need "no cli checkout at NSELF (internal/compose/images.lock.json missing)"
CLI="$(cd "$CLI" && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  if [ "$(id -u)" = 0 ] && command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq docker.io >/dev/null 2>&1 || true
  fi
fi
for tool in docker go openssl tar awk; do
  command -v "$tool" >/dev/null 2>&1 || need "missing $tool"
done
docker info >/dev/null 2>&1 || need "Docker unavailable"
[ "$(docker info --format '{{.OSType}}' 2>/dev/null)" = linux ] || need "Docker daemon is not running Linux containers"
case "$(docker info --format '{{.Architecture}}' 2>/dev/null)" in
  aarch64|arm64) GOARCH_TARGET=arm64 ;;
  x86_64|amd64) GOARCH_TARGET=amd64 ;;
  *) need "unsupported Docker daemon architecture" ;;
esac

WORK="$(mktemp -d "${TMPDIR:-/tmp}/minio-migrate.XXXXXX")"
SIBLING="p57-dind-$(openssl rand -hex 6)"
DIND_IMAGE='docker:28-dind@sha256:2a232a42256f70d78e3cc5d2b5d6b3276710a0de0596c145f627ecfae90282ac'
cleanup() {
  docker rm -f -v "$SIBLING" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# --- image pin: fragment, manifest and the cli image lock must name one digest -------------------------------
FRAG="$PLUGIN_DIR/docker-compose.plugin.yml"
FRAG_IMAGE="$(awk '$1 == "image:" { print $2; exit }' "$FRAG")"
FRAG_DIGEST="${FRAG_IMAGE##*@}"
MANIFEST_IMAGE="$(awk -F'"' '/"image":/ { print $4; exit }' "$PLUGIN_DIR/plugin.json")"
LOCK_DIGEST="$(awk '/"name": "minio"/ { f = 1 } f && /"index_digest"/ { gsub(/[",]/, "", $2); print $2; exit }' "$CLI/internal/compose/images.lock.json")"
case "$FRAG_IMAGE" in docker.io/pgsty/minio:*@sha256:*) ;; *) fail "fragment image is not a tag plus digest pgsty/minio reference" ;; esac
[ "$MANIFEST_IMAGE" = "$FRAG_IMAGE" ] || fail "plugin.json service.image differs from the fragment image"
[ -n "$LOCK_DIGEST" ] || fail "minio index_digest not found in the cli image lock"
[ "$FRAG_DIGEST" = "$LOCK_DIGEST" ] || fail "image digest drift: fragment differs from the cli image lock (minio.index_digest)"
echo 'PASS: fragment, plugin.json and cli image lock name the same minio digest'

# --- build the CLI (static, for the daemon's architecture) ----------------------------------------------------
version_lt() { # true when dotted version $1 is older than $2 (missing parts are 0)
  awk -v a="$1" -v b="$2" 'BEGIN {
    na = split(a, x, "."); nb = split(b, y, ".");
    for (i = 1; i <= 3; i++) {
      if (x[i] + 0 < y[i] + 0) exit 0
      if (x[i] + 0 > y[i] + 0) exit 1
    }
    exit 1 }'
}
GO_HAVE="$(GOTOOLCHAIN=local go env GOVERSION | sed 's/^go//')"
GO_NEED="$(awk '$1 == "go" { print $2; exit }' "$CLI/go.mod")"
if version_lt "$GO_HAVE" "$GO_NEED"; then
  export GOTOOLCHAIN=auto
  echo "INFO: go $GO_HAVE is older than the cli go.mod (go $GO_NEED); GOTOOLCHAIN=auto fetches the required toolchain"
fi
(cd "$CLI" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH_TARGET" go build -mod=vendor -o "$WORK/nself" ./cmd/nself) || fail "CLI build"
echo "PASS: CLI built from $(basename "$CLI") for linux/$GOARCH_TARGET"

# --- local plugin archive (what a plugin URL install downloads), without tests ----------------------------------
tar -czf "$WORK/minio.tgz" -C "$PLUGIN_DIR" --exclude=./test . || fail "plugin archive"
if command -v sha256sum >/dev/null 2>&1; then ARCHIVE_SUM="$(sha256sum "$WORK/minio.tgz" | cut -d' ' -f1)"
else ARCHIVE_SUM="$(shasum -a 256 "$WORK/minio.tgz" | cut -d' ' -f1)"; fi

# --- synthetic credentials, 0600 -----------------------------------------------------------------------------
umask 077
{
  printf 'MINIO_ROOT_USER=p57%s\n' "$(openssl rand -hex 5)"
  printf 'MINIO_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 16)"
  printf 'ARCHIVE_SUM=%s\n' "$ARCHIVE_SUM"
  printf 'EXPECT_IMAGE=%s\n' "$FRAG_IMAGE"
} >"$WORK/run.env"
umask 022

# --- isolated daemon and the flow ---------------------------------------------------------------------------
docker run -d --privileged --name "$SIBLING" -e DOCKER_TLS_CERTDIR= "$DIND_IMAGE" >/dev/null || fail "dind start"
n=0
until docker exec "$SIBLING" docker info >/dev/null 2>&1; do
  n=$((n + 1)); [ "$n" -le 60 ] || fail "dind daemon timeout"
  sleep 1
done
docker exec "$SIBLING" apk add --no-cache bash openssl busybox-extras >/dev/null || fail "dind dependencies"
docker exec "$SIBLING" mkdir -p /work
docker cp "$WORK/nself" "$SIBLING:/work/nself"
docker cp "$WORK/minio.tgz" "$SIBLING:/work/minio.tgz"
docker cp "$WORK/run.env" "$SIBLING:/work/run.env"
docker cp "$HERE/lib-sibling.sh" "$SIBLING:/work/lib-sibling.sh"
docker exec "$SIBLING" chmod 600 /work/run.env
docker exec "$SIBLING" bash /work/lib-sibling.sh || fail "MinIO data continuity flow"
echo PASS
