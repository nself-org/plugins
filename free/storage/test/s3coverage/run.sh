#!/usr/bin/env bash
# s3coverage runner: start a pinned S3 server image plus Postgres in docker, run the S3 coverage suite
# (free/storage/test/s3coverage, build tag s3coverage) against it from a pinned golang container, tear everything down.
#
# Usage:   TMPDIR=/some/scratch bash run.sh seaweedfs|minio
# Needs:   docker and openssl on the host (nothing else). TMPDIR must be set (scratch work dir lives under it).
# Secrets: random per run; they reach containers only through files (docker --env-file mode 600, the SeaweedFS
#          identity JSON bind-mounted read-only); they are never on a docker or go command line.
# Output:  resolved image digests, one PASS/FAIL line per TestOp_*/TestPlugin_*, then
#          "s3coverage <image>: ok|FAIL (n passed, m failed)". Exit 1 on any failure.
set -euo pipefail

# Pinned images (OCI index digests). Change a pin only together with the image lock.
SEAWEEDFS_IMAGE='chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d'
MINIO_IMAGE='pgsty/minio:RELEASE.2026-08-04T00-00-00Z@sha256:b6bfe7239bfc83fb90d31612d9704d86039dd714f7904b3f1ad68f211e602372'
RUNNER_IMAGE='golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade'
DB_IMAGE='postgres:16@sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d'

KIND="${1:-}"
case "$KIND" in
  seaweedfs) S3_IMAGE="$SEAWEEDFS_IMAGE"; S3_PORT=8333 ;;
  minio)     S3_IMAGE="$MINIO_IMAGE";     S3_PORT=9000 ;;
  *) echo "usage: run.sh seaweedfs|minio" >&2; exit 2 ;;
esac
: "${TMPDIR:?TMPDIR must be set}"
command -v docker >/dev/null || { echo "run.sh: docker is required" >&2; exit 2; }
command -v openssl >/dev/null || { echo "run.sh: openssl is required" >&2; exit 2; }

MODROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"   # free/storage
WORK="$(mktemp -d "${TMPDIR}/s3cov.XXXXXX")"
RUN_ID="s3cov-$(openssl rand -hex 4)"
NET="$RUN_ID"
C_S3="$RUN_ID-s3"
C_DB="$RUN_ID-db"

cleanup() {
  docker rm -f -v "$C_S3" "$C_DB" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

ACCESS_KEY="$(openssl rand -hex 8)"
SECRET_KEY="$(openssl rand -hex 16)"
DB_PASS="$(openssl rand -hex 16)"

umask 077
# Runner environment (credentials never on argv).
cat >"$WORK/runner.env" <<ENVEOF
S3COV_ENDPOINT=http://s3:${S3_PORT}
S3COV_ACCESS_KEY=${ACCESS_KEY}
S3COV_SECRET_KEY=${SECRET_KEY}
S3COV_REGION=us-east-1
S3COV_TARGET=${KIND}
S3COV_DB_URL=postgres://s3cov:${DB_PASS}@db:5432/s3cov?sslmode=disable
ENVEOF
cat >"$WORK/db.env" <<ENVEOF
POSTGRES_USER=s3cov
POSTGRES_PASSWORD=${DB_PASS}
POSTGRES_DB=s3cov
ENVEOF

echo "== images"
for img in "$S3_IMAGE" "$DB_IMAGE" "$RUNNER_IMAGE"; do
  docker image inspect "$img" >/dev/null 2>&1 || docker pull -q "$img" >/dev/null
  printf '%s\n   digests: ' "$img"
  docker image inspect --format '{{json .RepoDigests}}' "$img"
done

docker network create "$NET" >/dev/null

# Postgres (alias db).
docker run -d --name "$C_DB" --network "$NET" --network-alias db --env-file "$WORK/db.env" "$DB_IMAGE" >/dev/null

# S3 server (alias s3).
if [ "$KIND" = seaweedfs ]; then
  cat >"$WORK/s3.json" <<JSONEOF
{"identities":[{"name":"s3cov-admin","credentials":[{"accessKey":"${ACCESS_KEY}","secretKey":"${SECRET_KEY}"}],"actions":["Admin","Read","Write","List","Tagging"]}]}
JSONEOF
  # The container user is not the host user; the file sits in a mode-700 work dir, so 644 here exposes it to nobody else.
  chmod 644 "$WORK/s3.json"
  docker run -d --name "$C_S3" --network "$NET" --network-alias s3 \
    -v "$WORK/s3.json:/etc/seaweedfs/s3.json:ro" "$S3_IMAGE" \
    server -dir=/data -s3 -s3.port=8333 -s3.config=/etc/seaweedfs/s3.json \
    -master.volumeSizeLimitMB=256 -volume.max=200 >/dev/null
else
  cat >"$WORK/minio.env" <<ENVEOF
MINIO_ROOT_USER=${ACCESS_KEY}
MINIO_ROOT_PASSWORD=${SECRET_KEY}
ENVEOF
  docker run -d --name "$C_S3" --network "$NET" --network-alias s3 --env-file "$WORK/minio.env" "$S3_IMAGE" server /data >/dev/null
fi

echo "== running the suite against $KIND"
rc=0
docker run --rm --network "$NET" --env-file "$WORK/runner.env" \
  -v "$MODROOT:/src:ro" -w /src \
  -e CGO_ENABLED=0 -e GOFLAGS=-mod=vendor -e GOCACHE=/tmp/gocache -e GOTMPDIR=/tmp -e GOTOOLCHAIN=local \
  "$RUNNER_IMAGE" \
  sh -c 'go test -mod=vendor -tags s3coverage -list "^Test(Op|Plugin)_" ./test/s3coverage/ >/tmp/list.txt 2>&1 || { cat /tmp/list.txt; exit 1; }
    echo "S3COV_EXPECTED=$(grep -c "^Test" /tmp/list.txt)"
    go test -mod=vendor -tags s3coverage -count=1 -v -timeout 25m ./test/s3coverage/' >"$WORK/out.txt" 2>&1 || rc=$?

grep -E '^(S3COV_EXPECTED|--- (PASS|FAIL|SKIP)|PASS$|FAIL|ok |panic|\s+[a-z0-9_]+\.go:[0-9]+:)' "$WORK/out.txt" || true
passed=$(grep -cE '^--- PASS: Test(Op|Plugin)_' "$WORK/out.txt" || true)
failed=$(grep -cE '^--- FAIL: Test' "$WORK/out.txt" || true)
# Declared-unsupported ops are logged by the tests as "declared unsupported: <target> <Op> (<Code>); ...".
declared=$(sed -n 's/.*declared unsupported: [a-z]* \([A-Za-z]*\) (.*/\1/p' "$WORK/out.txt" | sort -u | paste -sd, -)
[ -n "$declared" ] || declared=none
# A deleted or renamed test must not go unnoticed: every TestOp_/TestPlugin_ function found by `go test -list` has to pass.
expected=$(sed -n 's/^S3COV_EXPECTED=//p' "$WORK/out.txt" | head -1)
expected=${expected:-0}
if [ "$rc" -ne 0 ] || [ "$failed" -ne 0 ] || [ "$expected" -eq 0 ] || [ "$passed" -lt "$expected" ]; then
  echo "== full test output"
  cat "$WORK/out.txt"
  echo "== $KIND server log (tail)"
  docker logs --tail 60 "$C_S3" 2>&1 || true
  echo "s3coverage $KIND: FAIL ($passed passed of $expected, $failed failed) declared-unsupported: $declared"
  exit 1
fi
echo "s3coverage $KIND: ok ($passed passed of $expected, $failed failed) declared-unsupported: $declared"
