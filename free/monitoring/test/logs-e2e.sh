#!/usr/bin/env bash
# logs-e2e.sh: boot the monitoring fragment on Linux Docker and prove the P7-PLUG-58 acceptance (ADR 0027, ADR 0030):
#   1. every service starts (healthy where it has a healthcheck);
#   2. no container is privileged and none mounts /var/run or docker.sock;
#   3. Alloy ships container logs to Loki: a Grafana datasource query for the plugin's own alloy container
#      returns lines within 60 s, and a tagged marker container is found by its container_name label;
#   4. cAdvisor, running without the socket, still reports container metrics to Prometheus.
# Needs: docker with compose v2 (plugin or standalone binary), curl, jq, a Linux daemon (Colima counts), network access to pull the pinned images, $TMPDIR.
# Fixtures: obviously fake credentials only. Host ports are ephemeral so the run never collides with a local stack.
# Exit: 0 pass, 1 an assertion failed (named), 2 a prerequisite is missing (a gate that cannot run is not a pass).
set -u
here=$(cd "$(dirname "$0")" && pwd)
plugin=$here/..
for t in docker curl jq; do command -v "$t" >/dev/null 2>&1 || { echo "logs-e2e: $t is required" >&2; exit 2; }; done
if docker compose version >/dev/null 2>&1; then compose=(docker compose)
elif command -v docker-compose >/dev/null 2>&1 && docker-compose version --short 2>/dev/null | sed 's/^v//' | awk -F. '{ exit !($1 + 0 >= 2) }'; then compose=(docker-compose)
else echo "logs-e2e: docker compose v2 (plugin or standalone) is required" >&2; exit 2; fi
[ "$(docker info --format '{{.OSType}}' 2>/dev/null)" = linux ] || { echo "logs-e2e: a Linux docker daemon is required" >&2; exit 2; }
: "${TMPDIR:?logs-e2e: TMPDIR must be set}"

work=$(mktemp -d "$TMPDIR/monitoring-e2e.XXXXXX")
proj=nsmone$$
rc=0
fail() { echo "FAIL $*"; rc=1; }
dc() { "${compose[@]}" -p "$proj" -f "$work/base.yml" -f "$work/monitoring.yml" "$@"; }
cleanup() { dc --profile redis down -v --remove-orphans >/dev/null 2>&1; docker rm -f "${proj}_marker" >/dev/null 2>&1; rm -rf "$work"; }
trap cleanup EXIT

# Render like the CLI: configs/ lands under ./monitoring in the project directory.
mkdir -p "$work/monitoring" && cp -R "$plugin/configs/." "$work/monitoring/"
sed -E 's/"127\.0\.0\.1:[0-9]+:([0-9]+)"/"127.0.0.1::\1"/' "$plugin/docker-compose.plugin.yml" > "$work/monitoring.yml"
cat > "$work/base.yml" <<EOF
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: e2e-fake-password
    networks: ["${proj}_net"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 3s
      timeout: 3s
      retries: 20
networks:
  ${proj}_net: {}
EOF
export COMPOSE_PROJECT_NAME=$proj DOCKER_NETWORK=${proj}_net POSTGRES_USER=postgres POSTGRES_PASSWORD=e2e-fake-password \
  POSTGRES_DB=postgres GRAFANA_ADMIN_USER=admin GRAFANA_ADMIN_PASSWORD=e2e-fake-grafana
cd "$work" || exit 2

echo "== pull and start"
dc config -q || { fail "compose config is invalid"; exit 1; }
dc --profile redis pull -q redis-exporter || fail "redis-exporter image (pinned digest) cannot be pulled"
dc up -d --wait --wait-timeout 240 || { fail "stack did not start healthy"; dc ps -a --format "table {{.Service}}\t{{.Status}}"; for s in $(dc ps -a --format "{{.Service}} {{.State}}" | awk '$2 != "running" { print $1 }'); do echo "--- $s"; dc logs --tail 25 "$s"; done; exit 1; }
for s in alloy cadvisor grafana loki prometheus tempo alertmanager node-exporter postgres-exporter; do
  [ -n "$(dc ps -q --status running "$s")" ] && echo "ok   $s running" || fail "$s is not running"
done
dc ps -a -q promtail | grep -q . && fail "a promtail container exists"

echo "== no privileged container, no socket mount"
inspected=0
for id in $(dc ps -q); do
  inspected=$((inspected + 1))
  name=$(docker inspect -f '{{.Name}}' "$id")
  [ "$(docker inspect -f '{{.HostConfig.Privileged}}' "$id")" = false ] || fail "$name is privileged"
  docker inspect -f '{{range .Mounts}}{{println .Source}}{{end}}' "$id" | grep -Eq '^/var/run(/|$)|docker\.sock' && fail "$name mounts /var/run or the Docker socket"
done
[ "$inspected" -ge 10 ] && echo "ok   inspected $inspected containers" || fail "inspected only $inspected containers (want the 9 monitoring services and postgres)"
grep -v '^[[:space:]]*//' "$plugin/configs/alloy/config.alloy" | grep -q 'discovery\.docker' && fail "alloy config uses discovery.docker"

echo "== logs reach Loki through Grafana"
gport=$(dc port grafana 3000 | sed 's/.*://')
gcurl() { curl -fsS -u "admin:$GRAFANA_ADMIN_PASSWORD" "$@"; }
for _ in $(seq 1 30); do gcurl "http://127.0.0.1:$gport/api/health" >/dev/null 2>&1 && break; sleep 1; done
uid=$(gcurl "http://127.0.0.1:$gport/api/datasources/name/Loki" | jq -r .uid)
[ -n "$uid" ] && [ "$uid" != null ] || { fail "Grafana has no Loki datasource"; exit 1; }
alloy_id=$(dc ps -q alloy)
docker run -d --name "${proj}_marker" --log-opt 'tag={{.Name}}|{{.ImageName}}' alpine:3.19 sh -c 'for i in 1 2 3 4 5 6; do echo e2e-marker-line; sleep 5; done' >/dev/null
loki_count() { # loki_count LOGQL: number of lines Grafana's Loki proxy returns for the last hour
  gcurl -G "http://127.0.0.1:$gport/api/datasources/proxy/uid/$uid/loki/api/v1/query_range" \
    --data-urlencode "query=$1" --data-urlencode limit=50 --data-urlencode "start=$(( $(date +%s) - 3600 ))000000000" 2>/dev/null |
    jq '[.data.result[]?.values | length] | add // 0'
}
t0=$(date +%s); own=0; mark=0; own_t=
while [ $(( $(date +%s) - t0 )) -lt 60 ]; do
  [ "${own:-0}" -gt 0 ] || { own=$(loki_count "{container_id=\"$alloy_id\"}"); [ "${own:-0}" -gt 0 ] && own_t=$(( $(date +%s) - t0 )); }
  mark=$(loki_count "{container_name=\"${proj}_marker\"} |= \"e2e-marker-line\"")
  [ "${own:-0}" -gt 0 ] && [ "${mark:-0}" -gt 0 ] && break; sleep 3
done
[ "${own:-0}" -gt 0 ] && echo "ok   alloy's own container: $own lines after $own_t s" || fail "no Loki lines for the alloy container within 60 s"
[ "${mark:-0}" -gt 0 ] && echo "ok   tagged marker container found by container_name: $mark lines" || fail "no Loki lines for the marker container (container_name label) within 60 s"

echo "== cAdvisor without the socket"
pport=$(dc port prometheus 9090 | sed 's/.*://')
q() { curl -fsS -G "http://127.0.0.1:$pport/api/v1/query" --data-urlencode "query=$1" | jq -r "$2"; }
t0=$(date +%s); up=0; series=0
while [ $(( $(date +%s) - t0 )) -lt 90 ]; do
  up=$(q 'up{job="cadvisor"}' '.data.result[0].value[1] // "0"'); series=$(q 'count(container_memory_usage_bytes)' '.data.result[0].value[1] // "0"')
  [ "$up" = 1 ] && [ "${series%%.*}" -gt 0 ] && break; sleep 5
done
[ "$up" = 1 ] && echo "ok   prometheus scrapes cadvisor" || fail "prometheus does not scrape cadvisor (up=$up)"
[ "${series%%.*}" -gt 0 ] && echo "ok   container_memory_usage_bytes: $series series" || fail "cadvisor reports no container metrics"
named=$(q 'count(container_memory_usage_bytes{name!=""})' '.data.result[0].value[1] // "0"')
echo "info container series carrying a name label (needs the Docker API; none is expected without the socket): $named"

[ $rc -eq 0 ] && echo "logs-e2e: PASS"
exit $rc
