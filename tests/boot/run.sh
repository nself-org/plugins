#!/usr/bin/env bash
# tests/boot/run.sh [--plugins a,b | --fixtures] [--sequential]: the fresh-DB boot kit. Builds the converted plugin
# image (network disabled for RUN steps), starts postgres:16, boots the plugin twice and requires: first boot
# applies all migrations, second applies none, and /health reports migrations applied == expected.
set -u
HERE=$(cd "$(dirname "$0")" && pwd); . "$HERE/../../scripts/conformance/lib.sh"
mode=fixtures; list=alpha
while [ $# -gt 0 ]; do case $1 in --plugins) mode=plugins; list=$2; shift;; --fixtures) mode=fixtures; list=alpha;; --sequential) ;; *) echo "unknown argument $1" >&2; exit 2;; esac; shift; done
W=$(conf_work)/boot; rm -rf "$W"; mkdir -p "$W"; rc=0; pid=$$; net=nself-boot-$pid; pg=nself-boot-pg-$pid
if [ "$mode" = fixtures ]; then bash "$CONF_ROOT/scripts/codemods/run.sh" --fixtures --out "$W/conv" >"$W/codemods.log" 2>&1 || { cat "$W/codemods.log"; exit 1; }; root=$W/conv; else root=$CONF_ROOT/free; fi
cleanup() { docker rm -f "$pg" "nself-boot-a-$pid" "nself-boot-b-$pid" >/dev/null 2>&1; docker network rm "$net" >/dev/null 2>&1; for p in $(printf '%s' "$list" | tr ',' ' '); do docker rmi -f "nself-boot-$p:$pid" >/dev/null 2>&1; done; }
trap cleanup EXIT
docker network create "$net" >/dev/null
docker run -d --name "$pg" --network "$net" -e POSTGRES_PASSWORD=boot -e POSTGRES_DB=boot postgres:16 >/dev/null
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U postgres -d boot >/dev/null 2>&1 && break; sleep 1; done
sleep 2
for p in $(printf '%s' "$list" | tr ',' ' '); do
  dir=$root/$p; pj=$dir/plugin.json; schema=$(jq -r '.schema // empty' "$pj"); port=$(jq -r '.port' "$pj")
  [ -n "$schema" ] || { echo "FAIL boot $p: manifest has no schema"; rc=1; continue; }
  docker build --network none -q -t "nself-boot-$p:$pid" "$dir" >/dev/null 2>"$W/build.err" || { echo "FAIL boot $p: image build failed"; tail -5 "$W/build.err"; rc=1; continue; }
  expected=$(ls "$dir"/migrations/*.sql 2>/dev/null | grep -vc '\.down\.sql$')
  docker exec "$pg" psql -U postgres -d boot -qc "CREATE SCHEMA IF NOT EXISTS $schema" >/dev/null
  for boot in a b; do
    c=nself-boot-$boot-$pid
    docker run -d --name "$c" --network "$net" -e DATABASE_URL="postgres://postgres:boot@$pg:5432/boot" -e PORT="$port" "nself-boot-$p:$pid" >/dev/null
    h=
    for i in $(seq 1 40); do h=$(docker exec "$c" wget -q -O- "http://127.0.0.1:$port/health" 2>/dev/null) && [ -n "$h" ] && break; sleep 1; done
    logs=$(docker logs "$c" 2>&1); docker rm -f "$c" >/dev/null
    applied=$(printf '%s' "$h" | jq -r '.migrations.applied // "none"' 2>/dev/null); exp=$(printf '%s' "$h" | jq -r '.migrations.expected // "none"' 2>/dev/null)
    want_log=$expected; [ $boot = b ] && want_log=0
    if printf '%s\n' "$logs" | grep -q "migrations: applied $want_log\$" && [ "$applied" = "$expected" ] && [ "$exp" = "$expected" ]; then
      echo "pass  boot $p #$boot: applied $want_log this boot, /health applied=$applied expected=$exp"
    else echo "FAIL  boot $p #$boot: want log 'applied $want_log' and /health $expected/$expected; got health='$h'"; printf '%s\n' "$logs" | tail -5; rc=1; fi
  done
done
[ $rc -eq 0 ] && echo "boot: ok" || echo "boot: FAIL"; exit $rc
