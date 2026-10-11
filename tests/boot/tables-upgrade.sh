#!/usr/bin/env bash
# tests/boot/tables-upgrade.sh --plugins a,b --basis-dir DIR [--sequential]: the tables-and-upgrade proof for boot-applied
# migrations (P7-PLUG-76; reused by P7-PLUG-77). Two passes per plugin, one plugin at a time:
#
#   Fresh pass    build the converted plugin (docker build --network none: nothing may need the network), boot it on an
#                 empty database and require /health migrations applied == expected and every `tables` entry of its
#                 manifest to exist at its schema in information_schema (`x` is public.x, `s.x` is schema s: the rule the
#                 cli tracking step applies).
#   Upgrade pass  build the BASIS tree (the plugin as it was before the conversion; default networking, because the basis
#                 Dockerfile still runs go mod download) from $BASIS/free/<plugin> (or $BASIS/<plugin>), boot it on an
#                 empty database, run free/<plugin>/tests/boot/legacy.sql when present (what an old install had that the
#                 basis build never created), then seed rows: free/<plugin>/tests/boot/seed.sql when present, else
#                 INSERT ... DEFAULT VALUES into every manifest table. A plugin that ends with zero seeded rows FAILS.
#                 Then boot the converted build on the SAME database and require applied == expected, every manifest
#                 table present, and every seeded row still there (by primary key).
#
# Basis image hook: free/<plugin>/tests/boot/basis-from (converted plugin dir; one line, an image ref) rewrites the image of
# the basis Dockerfile's first `FROM golang:` line (an `AS name` is kept) to that ref before the basis image is built, and
# prints a line saying so. For a basis tree that never built as shipped (storage: go.mod needs go >= 1.25, FROM golang:1.22);
# everything else of the basis build stays as shipped. Test-only.
# Row survival is checked by identity, not count: the primary-key values of every seeded row are saved before the upgrade
# and each must still exist after the converted boot (a table without a primary key falls back to its row count).
# Env hook, same as tests/boot/run.sh: free/<plugin>/tests/boot/env holds KEY=value lines passed as -e to every boot;
# only $DATABASE_URL and $PORT (or ${...}) are expanded, by string replacement (no eval); dummy values only.
# Basis tree: git archive "$(git merge-base origin/main HEAD)" free/<plugin>... | tar -x -C .boot-basis (never committed).
# Output: one "pass"/"FAIL" line per check; the LAST line is "tables-upgrade: ok" or "tables-upgrade: FAIL".
# Cleans every container, network and image it made. Needs docker, jq. Bash 3.2 compatible.
set -u
HERE=$(cd "$(dirname "$0")" && pwd); . "$HERE/../../scripts/conformance/lib.sh"
list=; basis=
while [ $# -gt 0 ]; do case $1 in
  --plugins) list=$2; shift;; --basis-dir) basis=$2; shift;; --sequential) ;;
  *) echo "unknown argument $1" >&2; exit 2;; esac; shift; done
[ -n "$list" ] && [ -n "$basis" ] || { echo "usage: tables-upgrade.sh --plugins a,b --basis-dir DIR [--sequential]" >&2; echo "tables-upgrade: FAIL"; exit 2; }
basis=$(cd "$basis" 2>/dev/null && pwd) || { echo "FAIL basis dir does not exist"; echo "tables-upgrade: FAIL"; exit 1; }
W=$(conf_work)/tables-upgrade; rm -rf "$W"; mkdir -p "$W"; rc=0; pid=$$; net=nself-tu-$pid; pg=nself-tu-pg-$pid; root=$CONF_ROOT/free

cleanup() {
  docker rm -f "$pg" "nself-tu-c-$pid" "nself-tu-b-$pid" >/dev/null 2>&1; docker network rm "$net" >/dev/null 2>&1
  for p in $(printf '%s' "$list" | tr ',' ' '); do docker rmi -f "nself-tu-$p:$pid" "nself-tu-basis-$p:$pid" >/dev/null 2>&1; done
}
trap cleanup EXIT

# sql <db> <statement>: one psql call, unaligned and tuples-only; non-zero on any SQL error.
sql() { docker exec "$pg" psql -U postgres -d "$1" -At -v ON_ERROR_STOP=1 -c "$2"; }
# sqlfile <db> <file>: run a SQL file quietly.
sqlfile() { docker exec -i "$pg" psql -U postgres -d "$1" -q -v ON_ERROR_STOP=1 < "$2"; }
# qualified <entry>: "schema.table" for a manifest tables entry (`x` is public.x, `s.x` is s.x).
qualified() { case $1 in *.*) printf '%s\n' "$1";; *) printf 'public.%s\n' "$1";; esac; }
# load_env <env file> <db url> <port>: fill the global envargs array from the env hook file.
load_env() {
  envargs=()
  [ -f "$1" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case $line in ''|'#'*) continue;; *=*) ;; *) continue;; esac
    line=${line//\$\{DATABASE_URL\}/$2}; line=${line//\$DATABASE_URL/$2}; line=${line//\$\{PORT\}/$3}; line=${line//\$PORT/$3}
    envargs+=(-e "$line")
  done < "$1"
}
# boot <container> <image> <db> <port>: start the image against <db> with the env hook; sets H (the /health body, empty
# when the plugin never answered) and LOGS (container logs); removes the container.
boot() {
  local c=$1 img=$2 db=$3 port=$4 i
  docker run -d --name "$c" --network "$net" -e DATABASE_URL="postgres://postgres:boot@$pg:5432/$db" -e PORT="$port" ${envargs[@]+"${envargs[@]}"} "$img" >/dev/null
  H=
  for i in $(seq 1 60); do
    H=$(docker exec "$c" wget -q -O- "http://127.0.0.1:$port/health" 2>/dev/null) && [ -n "$H" ] && break
    H=; [ "$(docker inspect -f '{{.State.Running}}' "$c" 2>/dev/null)" = true ] || break
    sleep 1
  done
  LOGS=$(docker logs "$c" 2>&1); docker rm -f "$c" >/dev/null
}
# mig_ok <expected>: 0 when H reports migrations applied == expected == <expected>.
mig_ok() {
  local a e; a=$(printf '%s' "$H" | jq -r '.migrations.applied // "none"' 2>/dev/null); e=$(printf '%s' "$H" | jq -r '.migrations.expected // "none"' 2>/dev/null)
  [ "$a" = "$1" ] && [ "$e" = "$1" ]
}
# tables_present <db> <tables...>: prints a FAIL line per missing table, returns 1 when any is missing.
tables_present() {
  local db=$1 t q n bad=0; shift
  for t in "$@"; do
    q=$(qualified "$t"); n=$(sql "$db" "SELECT count(*) FROM information_schema.tables WHERE table_schema = '${q%%.*}' AND table_name = '${q#*.}'" 2>/dev/null)
    [ "$n" = 1 ] || { echo "FAIL  $p: manifest table $t is not at ${q%%.*} (found $n)"; bad=1; }
  done
  return $bad
}
# counts <db> <tables...>: "<count> " per table, in order ("x " for a table that cannot be read).
counts() {
  local db=$1 t n out=; shift
  for t in "$@"; do n=$(sql "$db" "SELECT count(*) FROM $(qualified "$t")" 2>/dev/null) || n=x; out="$out${n:-x} "; done
  printf '%s\n' "$out"
}

# pkrows <db> <table> <file>: the primary-key values of every row ("(v1,v2)" text), sorted, into <file>. Returns 1 when the
# table has no primary key (the caller then falls back to the row count).
pkrows() {
  local q cols
  q=$(qualified "$2")
  cols=$(sql "$1" "SELECT string_agg(format('%I', a.attname), ',' ORDER BY k.ord) FROM pg_index i CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY k(attnum, ord) JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum WHERE i.indrelid = '$q'::regclass AND i.indisprimary" 2>/dev/null)
  [ -n "$cols" ] || return 1
  sql "$1" "SELECT ROW($cols)::text FROM $q" 2>/dev/null | LC_ALL=C sort > "$3"
}

docker network create "$net" >/dev/null
docker run -d --name "$pg" --network "$net" -e POSTGRES_PASSWORD=boot -e POSTGRES_DB=boot postgres:16 >/dev/null
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U postgres -d boot >/dev/null 2>&1 && break; sleep 1; done
sleep 2

for p in $(printf '%s' "$list" | tr ',' ' '); do
  dir=$root/$p; pj=$dir/plugin.json; [ -f "$pj" ] || { echo "FAIL  $p: no plugin.json under $root"; rc=1; continue; }
  schema=$(jq -r '.schema // empty' "$pj"); port=$(jq -r '.port' "$pj"); tables=$(jq -r '.tables[]?' "$pj" | tr '\n' ' ')
  [ -n "$schema" ] || { echo "FAIL  $p: manifest has no schema"; rc=1; continue; }
  [ -n "$tables" ] || { echo "FAIL  $p: manifest has no tables (nothing to prove)"; rc=1; continue; }
  expected=$(ls "$dir"/migrations/*.sql 2>/dev/null | grep -vc '\.down\.sql$'); tag=nself-tu-$p:$pid; btag=nself-tu-basis-$p:$pid
  safe=$(printf '%s' "$p" | tr '-' '_'); fdb=tu_f_$safe; udb=tu_u_$safe
  durl_f="postgres://postgres:boot@$pg:5432/$fdb"; durl_u="postgres://postgres:boot@$pg:5432/$udb"

  # ---- fresh pass
  docker build --network none -q -t "$tag" "$dir" >/dev/null 2>"$W/build.err" || { echo "FAIL  fresh $p: converted image build failed"; tail -5 "$W/build.err"; rc=1; continue; }
  sql boot "CREATE DATABASE $fdb" >/dev/null && sql "$fdb" "CREATE SCHEMA IF NOT EXISTS $schema" >/dev/null
  load_env "$dir/tests/boot/env" "$durl_f" "$port"; boot "nself-tu-c-$pid" "$tag" "$fdb" "$port"
  if mig_ok "$expected" && tables_present "$fdb" $tables; then echo "pass  fresh $p: applied=expected=$expected, $(printf '%s' "$tables" | wc -w | tr -d ' ') manifest tables at their schema"
  else echo "FAIL  fresh $p: want migrations $expected/$expected and every manifest table present; health='$H'"; printf '%s\n' "$LOGS" | tail -5; rc=1; continue; fi

  # ---- upgrade pass
  bdir=$basis/free/$p; [ -d "$bdir" ] || bdir=$basis/$p
  [ -f "$bdir/Dockerfile" ] || { echo "FAIL  upgrade $p: no basis tree (Dockerfile) under $basis"; rc=1; continue; }
  bctx=$bdir; bf=$dir/tests/boot/basis-from
  if [ -f "$bf" ]; then
    ref=$(head -1 "$bf" | tr -d '[:space:]'); bctx=$W/basis-$p; rm -rf "$bctx"; mkdir -p "$bctx"; cp -R "$bdir/." "$bctx/"
    awk -v ref="$ref" '!done && /^FROM golang:/ { n = split($0, a, " "); out = "FROM " ref; for (i = 3; i <= n; i++) out = out " " a[i]; print out; done = 1; next } { print }' "$bdir/Dockerfile" > "$bctx/Dockerfile"
    echo "note  upgrade $p: basis Dockerfile first FROM golang: line rewritten to $ref (tests/boot/basis-from)"
  fi
  docker build -q -t "$btag" "$bctx" >/dev/null 2>"$W/build.err" || { echo "FAIL  upgrade $p: basis image build failed"; tail -5 "$W/build.err"; rc=1; continue; }
  sql boot "CREATE DATABASE $udb" >/dev/null
  load_env "$dir/tests/boot/env" "$durl_u" "$port"; boot "nself-tu-b-$pid" "$btag" "$udb" "$port"
  if [ -z "$H" ]; then echo "FAIL  upgrade $p: the basis build never answered /health on a fresh database (record it and escalate)"; printf '%s\n' "$LOGS" | tail -8; rc=1; continue; fi
  if [ -f "$dir/tests/boot/legacy.sql" ]; then sqlfile "$udb" "$dir/tests/boot/legacy.sql" || { echo "FAIL  upgrade $p: legacy.sql failed"; rc=1; continue; }; fi
  if [ -f "$dir/tests/boot/seed.sql" ]; then sqlfile "$udb" "$dir/tests/boot/seed.sql" || { echo "FAIL  upgrade $p: seed.sql failed"; rc=1; continue; }
  else for t in $tables; do sql "$udb" "INSERT INTO $(qualified "$t") DEFAULT VALUES" >/dev/null 2>&1; done; fi
  before=$(counts "$udb" $tables); seeded=0; unreadable=0
  for n in $before; do case $n in x) unreadable=1;; *) seeded=$((seeded + n));; esac; done
  if [ $seeded -lt 1 ] || [ $unreadable -eq 1 ]; then echo "FAIL  upgrade $p: seeded rows before the upgrade: counts '$before' (need at least one row, every manifest table readable)"; rc=1; continue; fi
  i=0; nopk=" "
  for t in $tables; do i=$((i + 1)); pkrows "$udb" "$t" "$W/pk-before-$i" || nopk="$nopk$i "; done
  boot "nself-tu-c-$pid" "$tag" "$udb" "$port"
  after=$(counts "$udb" $tables); lost=0; i=0
  for t in $tables; do
    i=$((i + 1)); n=$(printf '%s' "$before" | awk -v k=$i '{ print $k }'); a=$(printf '%s' "$after" | awk -v k=$i '{ print $k }')
    case $nopk in
      *" $i "*) case $a in x|'') lost=1;; *) [ "$a" -ge "$n" ] || { lost=1; echo "FAIL  upgrade $p: table $t lost rows (count $n -> $a)"; };; esac;;
      *) if pkrows "$udb" "$t" "$W/pk-after-$i" && [ -z "$(LC_ALL=C comm -23 "$W/pk-before-$i" "$W/pk-after-$i")" ]; then :; else lost=1; echo "FAIL  upgrade $p: table $t lost or rewrote a seeded row (primary keys: $(LC_ALL=C comm -23 "$W/pk-before-$i" "$W/pk-after-$i" 2>/dev/null | head -3 | tr '\n' ' '))"; fi;;
    esac
  done
  if mig_ok "$expected" && tables_present "$udb" $tables && [ $lost -eq 0 ]; then echo "pass  upgrade $p: basis seeded $seeded row(s) (counts $before), converted boot applied=expected=$expected, counts after $after"
  else echo "FAIL  upgrade $p: want migrations $expected/$expected, tables present, no row lost; health='$H' counts before '$before' after '$after'"; printf '%s\n' "$LOGS" | tail -8; rc=1; fi
  docker rmi -f "$tag" "$btag" >/dev/null 2>&1
done
[ $rc -eq 0 ] && echo "tables-upgrade: ok" || echo "tables-upgrade: FAIL"; exit $rc
