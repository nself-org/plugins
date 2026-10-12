#!/usr/bin/env bash
# Purpose: the in-sibling half of migrate-from-core.sh. Runs inside the isolated dind container.
# Usage:   bash /work/lib-sibling.sh   (migrate-from-core.sh copies it in; do not run it on a developer machine)
# Env:     reads /work/run.env (synthetic MinIO root credentials, archive checksum, expected image reference).
# Needs:   /work/nself (static CLI), /work/minio.tgz (plugin archive), docker with compose, bash, openssl, httpd (busybox-extras).
# Outputs: PASS lines per check; exits 1 with a FAIL line on the first failure.
# Constraints: bash 3.2 compatible. Credentials stay in files; the S3 client gets them through docker --env-file.
#          Only the minio service is started (docker compose up --no-deps over the build's recorded files), not the whole stack.
set -euo pipefail
umask 022 # known CLI debt D-0664: nself build fails under umask 077 (E452); secret files get chmod 600 below
export DOCKER_HOST=unix:///var/run/docker.sock HOME=/work/home NSELF_PLUGIN_DIR=/work/plugins
NSELF=/work/nself
mkdir -p "$HOME" "$NSELF_PLUGIN_DIR" /work/project /work/serve
fail() { echo "FAIL: $1"; exit 1; }
# shellcheck disable=SC1091 # copied per-run file; sourced without export so the CLI never inherits the credentials
. /work/run.env
cd /work/project

run_nself() { # run_nself LOG ARGS...: run the CLI, show the log tail and fail on a non-zero exit
  local log=$1; shift
  "$NSELF" "$@" >"$log" 2>&1 || { tail -40 "$log"; fail "nself $1"; }
}
# `nself service start` runs compose without the project env files (empty MinIO credentials, so MinIO falls back to
# minioadmin: a CLI gap reported with this Ticket). Start the way nself start does: the files the build recorded.
up_minio() { # up_minio LOG: docker compose up -d --no-deps minio over .nself/compose-files.txt and compose-env-files.txt
  local args=() f
  while IFS= read -r f; do [ -n "$f" ] && args+=(--env-file "$f"); done <.nself/compose-env-files.txt
  while IFS= read -r f; do [ -n "$f" ] && args+=(-f "$f"); done <.nself/compose-files.txt
  docker compose "${args[@]}" up -d --no-deps minio >"$1" 2>&1 || { tail -40 "$1"; fail "compose up minio"; }
}
minio_ctr() { docker ps -aq --filter label=com.docker.compose.service=minio | head -1; }
wait_healthy() { # wait_healthy CONTAINER
  local n=0
  until [ "$(docker inspect -f '{{.State.Health.Status}}' "$1")" = healthy ]; do
    n=$((n + 1)); [ "$n" -le 90 ] || fail "minio health timeout"
    sleep 2
  done
}
# A separate S3 client: the pinned image run as mc in the minio container's network namespace (127.0.0.1:9000).
# Uploads go as files from /work/io, never through docker exec stdin (an exec -i pipe failed at random, no error).
mc_run() { docker run --rm --network "container:$CTR" -v /work/io:/io:ro --env-file /work/mc.env --entrypoint mc "$EXPECT_IMAGE" --quiet --config-dir /tmp/mc-config "$@"; }
sha_of_object() { mc_run cat "p57/uploads/plug57/$1" | sha256sum | cut -d' ' -f1; }
core_env() { # the MinIO settings the container actually got, sorted (compared, never printed)
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$1" \
    | grep -E '^MINIO_(ROOT_USER|ROOT_PASSWORD|DEFAULT_BUCKETS|DISABLE_XATTR)=' | LC_ALL=C sort
}
volume_of() { docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$1"; }

# ---- phase 1: v1.4 mode, core MinIO ---------------------------------------------------------------------------
run_nself /work/init.log init --non-interactive --name p57mig --no-pgvector
if grep -q '^MINIO_ENABLED=' .env; then sed -i 's/^MINIO_ENABLED=.*/MINIO_ENABLED=true/' .env; else printf 'MINIO_ENABLED=true\n' >>.env; fi
touch .env.secrets; chmod 600 .env.secrets
printf 'MINIO_ROOT_USER=%s\nMINIO_ROOT_PASSWORD=%s\n' "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >>.env.secrets
printf 'MC_HOST_p57=http://%s:%s@127.0.0.1:9000\n' "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/work/mc.env
chmod 600 /work/mc.env
run_nself /work/build-v14.log build
grep -Eq '^ +minio:$' docker-compose.yml || fail "v1.4 build did not generate core minio"
up_minio /work/start-v14.log
CTR="$(minio_ctr)"; [ -n "$CTR" ] || fail "v1.4 minio container missing"
wait_healthy "$CTR"
VOL="$(volume_of "$CTR")"; [ -n "$VOL" ] || fail "v1.4 minio has no /data volume"
VOL_CREATED="$(docker volume inspect -f '{{.CreatedAt}}' "$VOL")"
core_env "$CTR" >/work/env-v14
echo "PASS: v1.4 core MinIO healthy on volume $VOL"

mc_run mb --ignore-existing p57/uploads >/dev/null || fail "bucket create"
mkdir -p /work/io
head -c 1048576 /dev/urandom >/work/io/payload.bin
printf 'continuity check\n' >/work/io/note.txt
want_payload="$(sha256sum /work/io/payload.bin | cut -d' ' -f1)"; want_note="$(sha256sum /work/io/note.txt | cut -d' ' -f1)"
mc_run cp /io/payload.bin p57/uploads/plug57/payload.bin >/work/put.log 2>&1 || { tail -5 /work/put.log; fail "put payload"; }
mc_run cp /io/note.txt p57/uploads/plug57/note.txt >/work/put.log 2>&1 || { tail -5 /work/put.log; fail "put note"; }
[ "$(sha_of_object payload.bin)" = "$want_payload" ] || fail "v1.4 read-back checksum (payload)"
[ "$(sha_of_object note.txt)" = "$want_note" ] || fail "v1.4 read-back checksum (note)"
echo 'PASS: object stored through S3 on core MinIO and read back'
OLD_CTR="$CTR"
docker rm -f "$OLD_CTR" >/dev/null || fail "remove v1.4 container"
docker volume inspect "$VOL" >/dev/null 2>&1 || fail "volume vanished with the container"

# ---- phase 2: v1.5 mode ---------------------------------------------------------------------------------------
export NSELF_V15=1
before="$(sha256sum docker-compose.yml | cut -d' ' -f1)"
set +e
"$NSELF" build >/work/e138.out 2>&1
code=$?
set -e
[ "$code" = 1 ] || { tail -20 /work/e138.out; fail "E138 exit was $code"; }
grep -q E138 /work/e138.out && grep -q 'nself add minio' /work/e138.out || fail "E138 remedy missing"
[ "$(sha256sum docker-compose.yml | cut -d' ' -f1)" = "$before" ] || fail "E138 changed docker-compose.yml"
echo 'PASS: v1.5 without the plugin stops with E138 and the remedy, files unchanged'

httpd -p 127.0.0.1:18099 -h /work/serve || fail "local archive server"
cp /work/minio.tgz /work/serve/minio.tgz
run_nself /work/add.log add http://127.0.0.1:18099/minio.tgz --yes --checksum "$ARCHIVE_SUM"
[ -f "$NSELF_PLUGIN_DIR/minio/plugin.json" ] && [ -f "$NSELF_PLUGIN_DIR/minio/docker-compose.plugin.yml" ] || fail "plugin files not installed"
echo 'PASS: plugin installed from the local archive (checksum verified)'

run_nself /work/build-v15.log build
grep -Eq '^ +minio:$' docker-compose.yml && fail "v1.5 core still generates minio"
up_minio /work/start-v15.log
CTR="$(minio_ctr)"; [ -n "$CTR" ] || fail "v1.5 minio container missing"
[ "$CTR" != "$OLD_CTR" ] || fail "container was not recreated"
wait_healthy "$CTR"
echo 'PASS: v1.5 plugin minio healthy'

[ "$(volume_of "$CTR")" = "$VOL" ] || fail "different data volume"
[ "$(docker volume inspect -f '{{.CreatedAt}}' "$VOL")" = "$VOL_CREATED" ] || fail "volume was recreated"
echo "PASS: same volume $VOL attached, not recreated"
[ "$(docker inspect -f '{{.Config.Image}}' "$CTR")" = "$EXPECT_IMAGE" ] || fail "container is not on the pinned image reference"
docker image inspect -f '{{join .RepoDigests " "}}' "$EXPECT_IMAGE" | grep -q "${EXPECT_IMAGE##*@}" || fail "pulled image digest differs"
echo 'PASS: container runs the pinned tag and digest'
core_env "$CTR" >/work/env-v15
cmp -s /work/env-v14 /work/env-v15 || fail "MinIO root and bucket settings differ from core (same .env)"
echo 'PASS: root credentials and settings equal core'

[ "$(sha_of_object payload.bin)" = "$want_payload" ] || fail "v1.5 read-back checksum (payload)"
[ "$(sha_of_object note.txt)" = "$want_note" ] || fail "v1.5 read-back checksum (note)"
echo 'PASS: both objects read back through S3 with the same checksums'

[ "$(docker inspect -f '{{.HostConfig.Privileged}}' "$CTR")" = false ] || fail "container is privileged"
[ "$(docker inspect -f '{{.HostConfig.ReadonlyRootfs}}' "$CTR")" = true ] || fail "root filesystem is not read-only"
docker inspect -f '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' "$CTR" | grep -q docker.sock && fail "docker socket mounted"
docker inspect -f '{{range .HostConfig.PortBindings}}{{range .}}{{.HostIp}}{{"\n"}}{{end}}{{end}}' "$CTR" | grep -v '^$' | grep -qvx 127.0.0.1 && fail "port not bound to loopback"
echo 'PASS: no socket, not privileged, read-only root, ports on 127.0.0.1'
