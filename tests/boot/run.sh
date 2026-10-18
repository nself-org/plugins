#!/usr/bin/env bash
# tests/boot/run.sh [--plugins a,b | --fixtures] [--sequential]: the fresh-DB boot kit. Builds the converted plugin
# image (network disabled for RUN steps), starts postgres:16 (pgvector/pgvector:pg16 when a plugin requires the vector extension), boots the plugin twice and requires: first boot
# applies all migrations, second applies none, and /health reports migrations applied == expected.
# The /health probe uses wget, or a deno fetch in images that have no wget; the migrations log line may be plain or a
# structured (JSON) log record.
# Env hook (P7-PLUG-76): when <plugin>/tests/boot/env exists, each KEY=value line is passed as -e to both boots. Only
# $DATABASE_URL and $PORT (or ${...}) are expanded, by plain string replacement (no eval); blank and # lines are
# skipped. Values are dummies for the boot, never real secrets. Plugins without the file boot exactly as before.
# Skipped: a plugin with a tests/boot/skip marker (first line = reason), or one whose manifest declares no migrations (and has no migrations/*.sql) (P7-PLUG-44).
# <plugin>/tests/boot/build-network (empty marker) lets the image build use the network (cargo fetch for the Rust plugin; Go plugins stay network-free).
set -u
HERE=$(cd "$(dirname "$0")" && pwd); . "$HERE/../../scripts/conformance/lib.sh"
mode=fixtures; list=alpha
while [ $# -gt 0 ]; do case $1 in --plugins) mode=plugins; list=$2; shift;; --fixtures) mode=fixtures; list=alpha;; --sequential) ;; *) echo "unknown argument $1" >&2; exit 2;; esac; shift; done
W=$(conf_work)/boot; rm -rf "$W"; mkdir -p "$W"; rc=0; pid=$$; net=nself-boot-$pid; pg=nself-boot-pg-$pid
if [ "$mode" = fixtures ]; then bash "$CONF_ROOT/scripts/codemods/run.sh" --fixtures --out "$W/conv" >"$W/codemods.log" 2>&1 || { cat "$W/codemods.log"; exit 1; }; root=$W/conv; else root=$CONF_ROOT/free; fi
cleanup() { docker rm -f "$pg" "nself-boot-a-$pid" "nself-boot-b-$pid" >/dev/null 2>&1; docker network rm "$net" >/dev/null 2>&1; for p in $(printf '%s' "$list" | tr ',' ' '); do docker rmi -f "nself-boot-$p:$pid" >/dev/null 2>&1; done; }
trap cleanup EXIT
docker network create "$net" >/dev/null
# Image: postgres:16, or pgvector/pgvector:pg16 when a plugin of the run requires the vector extension (manifest requires.postgres_extensions).
pgimg=postgres:16
for p in $(printf '%s' "$list" | tr ',' ' '); do
  jq -e '(.requires.postgres_extensions // []) | index("vector")' "$root/$p/plugin.json" >/dev/null 2>&1 && pgimg=pgvector/pgvector:pg16
done
docker run -d --name "$pg" --network "$net" -e POSTGRES_PASSWORD=boot -e POSTGRES_DB=boot "$pgimg" >/dev/null
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U postgres -d boot >/dev/null 2>&1 && break; sleep 1; done
sleep 2
for p in $(printf '%s' "$list" | tr ',' ' '); do
  dir=$root/$p; pj=$dir/plugin.json; schema=$(jq -r '.schema // empty' "$pj"); port=$(jq -r '.port' "$pj")
  # Nothing to boot: a marker file tests/boot/skip whose first line says why (a library or hook binary with no HTTP server), or a plugin that declares no migrations at all.
  # A manifest with a migrations key but no SQL still boots (and fails on its own terms).
  [ -f "$dir/tests/boot/skip" ] && { echo "skip  boot $p: $(head -1 "$dir/tests/boot/skip")"; continue; }
  if ! ls "$dir"/migrations/*.sql >/dev/null 2>&1 && [ "$(jq -r '.migrations // empty | tostring' "$pj")" = "" ]; then echo "skip  boot $p: no migrations declared (nothing to apply)"; continue; fi
  [ -n "$schema" ] || { echo "FAIL boot $p: manifest has no schema"; rc=1; continue; }
  netflag="--network none"; [ -f "$dir/tests/boot/build-network" ] && netflag=   # Rust cargo fetch (P7-PLUG-44 ruling): the marker file lifts the network ban for the image build only
  docker build $netflag -q -t "nself-boot-$p:$pid" "$dir" >/dev/null 2>"$W/build.err" || { echo "FAIL boot $p: image build failed"; tail -5 "$W/build.err"; rc=1; continue; }
  dburl="postgres://postgres:boot@$pg:5432/boot"; envargs=(); ef=$dir/tests/boot/env
  if [ -f "$ef" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case $line in ''|'#'*) continue;; *=*) ;; *) continue;; esac
      line=${line//\$\{DATABASE_URL\}/$dburl}; line=${line//\$DATABASE_URL/$dburl}; line=${line//\$\{PORT\}/$port}; line=${line//\$PORT/$port}
      envargs+=(-e "$line")
    done < "$ef"
  fi
  expected=$(ls "$dir"/migrations/*.sql 2>/dev/null | grep -vc '\.down\.sql$')
  docker exec "$pg" psql -U postgres -d boot -qc "CREATE SCHEMA IF NOT EXISTS $schema" >/dev/null
  for boot in a b; do
    c=nself-boot-$boot-$pid
    docker run -d --name "$c" --network "$net" -e DATABASE_URL="$dburl" -e PORT="$port" ${envargs[@]+"${envargs[@]}"} "nself-boot-$p:$pid" >/dev/null
    h=
    for i in $(seq 1 40); do h=$(if docker exec "$c" sh -c 'command -v wget' >/dev/null 2>&1; then docker exec "$c" wget -q -O- "http://127.0.0.1:$port/health"; else docker exec "$c" deno eval "console.log(await (await fetch('http://127.0.0.1:$port/health')).text())"; fi 2>/dev/null) && [ -n "$h" ] && break; sleep 1; done
    logs=$(docker logs "$c" 2>&1); docker rm -f "$c" >/dev/null
    applied=$(printf '%s' "$h" | jq -r '.migrations.applied // "none"' 2>/dev/null); exp=$(printf '%s' "$h" | jq -r '.migrations.expected // "none"' 2>/dev/null)
    want_log=$expected; [ $boot = b ] && want_log=0
    if printf '%s\n' "$logs" | grep -Eq "migrations: applied $want_log(\"[,}].*)?\$" && [ "$applied" = "$expected" ] && [ "$exp" = "$expected" ]; then
      echo "pass  boot $p #$boot: applied $want_log this boot, /health applied=$applied expected=$exp"
    else echo "FAIL  boot $p #$boot: want log 'applied $want_log' and /health $expected/$expected; got health='$h'"; printf '%s\n' "$logs" | tail -5; rc=1; fi
  done
done
[ $rc -eq 0 ] && echo "boot: ok" || echo "boot: FAIL"; exit $rc
