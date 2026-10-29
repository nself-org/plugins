#!/usr/bin/env bash
# tests/boot/tables-upgrade.sh --plugins a,b --basis-dir DIR [--sequential]: the tables-and-upgrade proof for boot-applied
# migrations (P7-PLUG-76; reused and widened by P7-PLUG-77). Three passes per plugin, one plugin at a time:
#
#   Fresh pass    build the converted plugin (docker build --network none: nothing may need the network), boot it on an
#                 empty database and require /health migrations applied == expected and every `tables` entry of its
#                 manifest to exist at its schema in information_schema (`x` is public.x, `s.x` is schema s: the rule the
#                 cli tracking step applies).
#   Legacy pass   (only for a plugin with free/<plugin>/tests/boot/legacy-renames; needs only the converted image) run
#                 free/<plugin>/tests/boot/legacy.sql (the TS-era unprefixed tables, with fixture rows) on an EMPTY database
#                 with no basis boot, boot the converted image and require migrations applied == expected, every manifest
#                 table present and, for each `old new` pair of legacy-renames: old gone, new present and every primary key
#                 captured from old before the boot present in new. legacy-renames without legacy.sql FAILS; a pair whose
#                 fixture is not true (old missing, new already there, old without a primary key) FAILS.
#   Upgrade pass  build the BASIS tree (the plugin as it was before the conversion; default networking, because the basis
#                 Dockerfile still runs go mod download) from $BASIS/free/<plugin> (or $BASIS/<plugin>), boot it on an
#                 empty database, run free/<plugin>/tests/boot/legacy.sql when present (what an old install had that the
#                 basis build never created), then seed rows: free/<plugin>/tests/boot/seed.sql when present, else
#                 INSERT ... DEFAULT VALUES into every manifest table. A plugin that ends with zero seeded rows FAILS.
#                 Then boot the converted build on the SAME database and require applied == expected, every manifest
#                 table present, and every seeded row still there (by primary key). Every table the basis creates needs at
#                 least one seeded row (FAIL naming the empty table). With legacy-renames, each pair is classified after
#                 legacy.sql and seed.sql: both tables exist (the boot must leave old with identical keys and new with its
#                 seeded keys) or only old exists (the boot must rename it, every key kept); one `upgrade-legacy` line per plugin. Each converted-boot log line that
#                 contains `legacy table` is printed as a `note  upgrade <plugin>: log: ...` line (legacy.sql run after the
#                 basis boot creates the both-exist case on purpose for plugins whose basis already had the np_ tables).
#
# Fixtures, all optional, under free/<plugin>/tests/boot/:
#   env             KEY=value lines passed to every boot (see the env hook below).
#   legacy.sql      TS-era unprefixed tables with fixture rows (fixed primary keys); used by the legacy and upgrade passes.
#   legacy-renames  `old new` lines naming public tables (`#` comments and blank lines skipped): the renames the legacy pass checks.
#   legacy-probe.sql  SQL run on the legacy database after the converted boot and the pair checks (legacy pass only; ON_ERROR_STOP,
#                   so a failing statement fails the pass): for a check the renames cannot express, e.g. that a
#                   renamed table still takes the plugin's inserts. Needs legacy-renames: conformance rule legacy-renames fails a
#                   probe that would never run. The pass line ends with `probe ok`.
#   seed.sql        rows written through the basis schema before the upgrade (parent rows first).
#   basis-from      image ref for the basis Dockerfile (see below).
#   basis-absent    manifest tables the BASIS build never created, one per line (a plugin whose conversion adds a table). The
#                   upgrade pass does not read, seed, count or capture them before the upgrade; the converted boot must still
#                   create them (they stay in the manifest table check). An entry that is not in the manifest, or that exists
#                   after the basis boot, FAILS: the fixture must be true.
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
n=0; for p in $(printf '%s' "$list" | tr ',' ' '); do n=$((n+1)); done
[ $n -gt 0 ] || { echo "FAIL --plugins '$list' names no plugin (a check that ran nothing)"; echo "tables-upgrade: FAIL"; exit 2; }
basis=$(cd "$basis" 2>/dev/null && pwd) || { echo "FAIL basis dir does not exist"; echo "tables-upgrade: FAIL"; exit 1; }
W=$(conf_work)/tables-upgrade; rm -rf "$W"; mkdir -p "$W"; rc=0; pid=$$; net=nself-tu-$pid; pg=nself-tu-pg-$pid; root=$CONF_ROOT/free

redis=nself-tu-redis-$pid
. "$HERE/redis.sh"

cleanup() {
  docker rm -f "$redis" "$pg" "nself-tu-c-$pid" "nself-tu-b-$pid" >/dev/null 2>&1; docker network rm "$net" >/dev/null 2>&1
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
# table has no primary key (the caller then falls back to the row count) and 2 when a query fails (the caller FAILs).
pkrows() {
  local q cols rows
  q=$(qualified "$2")
  cols=$(sql "$1" "SELECT coalesce(string_agg(format('%I', a.attname), ',' ORDER BY k.ord), '') FROM pg_index i CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY k(attnum, ord) JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum WHERE i.indrelid = '$q'::regclass AND i.indisprimary" 2>/dev/null) || return 2
  [ -n "$cols" ] || return 1
  rows=$(sql "$1" "SELECT ROW($cols)::text FROM $q" 2>/dev/null) || return 2
  if [ -n "$rows" ]; then
    printf '%s\n' "$rows" | LC_ALL=C sort > "$3"
  else
    : > "$3"
  fi
}

# legacy_pass: the LEGACY pass for the current plugin (globals p dir tag schema port tables expected safe W pid pg). Silent no-op
# without tests/boot/legacy-renames; 0 on pass, 1 on any FAIL (the caller keeps going to the upgrade pass).
legacy_pass() {
  local lr=$dir/tests/boot/legacy-renames ldb=tu_l_$safe line old new extra np=0 nrows=0 bad=0 r k cnt lurl probe=
  [ -f "$lr" ] || return 0
  [ -f "$dir/tests/boot/legacy.sql" ] || { echo "FAIL  legacy $p: legacy-renames without legacy.sql"; return 1; }
  lurl="postgres://postgres:boot@$pg:5432/$ldb"; : > "$W/lpairs-$p"
  sql boot "CREATE DATABASE $ldb" >/dev/null && sql "$ldb" "CREATE SCHEMA IF NOT EXISTS $schema" >/dev/null
  sqlfile "$ldb" "$dir/tests/boot/legacy.sql" || { echo "FAIL  legacy $p: legacy.sql failed on the empty database"; return 1; }
  while IFS= read -r line || [ -n "$line" ]; do
    case $line in ''|'#'*) continue;; esac
    old=; new=; extra=; read -r old new extra <<EOF
$line
EOF
    if [ -z "$old" ] || [ -z "$new" ] || [ -n "$extra" ]; then echo "FAIL  legacy $p: legacy-renames line '$line' is not 'old new'"; bad=1; continue; fi
    np=$((np + 1))
    if [ "$(sql "$ldb" "SELECT to_regclass('public.$old') IS NOT NULL AND to_regclass('public.$new') IS NULL" 2>/dev/null)" != t ]; then
      echo "FAIL  legacy $p: fixture must create public.$old and not public.$new (legacy.sql ran on an empty database)"; bad=1; continue; fi
    pkrows "$ldb" "$old" "$W/lpk-before-$np"; r=$?
    if [ $r -ne 0 ]; then echo "FAIL  legacy $p: fixture table $old has no primary key or its keys cannot be read (fixtures must have PKs)"; bad=1; continue; fi
    cnt=$(wc -l < "$W/lpk-before-$np" | tr -d ' ')
    if [ "$cnt" -lt 1 ]; then echo "FAIL  legacy $p: fixture table $old has no rows"; bad=1; continue; fi
    nrows=$((nrows + cnt)); printf '%s %s %s\n' "$np" "$old" "$new" >> "$W/lpairs-$p"
  done < "$lr"
  [ $np -gt 0 ] || { echo "FAIL  legacy $p: legacy-renames names no pair"; return 1; }
  [ $bad -eq 0 ] || return 1
  load_env "$dir/tests/boot/env" "$lurl" "$port"; boot "nself-tu-c-$pid" "$tag" "$ldb" "$port"
  if ! { mig_ok "$expected" && tables_present "$ldb" $tables; }; then
    echo "FAIL  legacy $p: converted boot on the legacy database: want migrations $expected/$expected and every manifest table present; health='$H'"; printf '%s\n' "$LOGS" | tail -8; return 1; fi
  while read -r k old new; do
    [ -n "$k" ] || continue
    if [ "$(sql "$ldb" "SELECT to_regclass('public.$old') IS NULL AND to_regclass('public.$new') IS NOT NULL" 2>/dev/null)" != t ]; then
      echo "FAIL  legacy $p: after the boot public.$old must be gone and public.$new present"; bad=1; continue; fi
    if pkrows "$ldb" "$new" "$W/lpk-after-$k" && [ -z "$(LC_ALL=C comm -23 "$W/lpk-before-$k" "$W/lpk-after-$k")" ]; then :
    else echo "FAIL  legacy $p: $new lost a fixture row of $old (missing keys: $(LC_ALL=C comm -23 "$W/lpk-before-$k" "$W/lpk-after-$k" 2>/dev/null | head -3 | tr '\n' ' '))"; bad=1; fi
  done < "$W/lpairs-$p"
  [ $bad -eq 0 ] || return 1
  if [ -f "$dir/tests/boot/legacy-probe.sql" ]; then
    sqlfile "$ldb" "$dir/tests/boot/legacy-probe.sql" >/dev/null || { echo "FAIL  legacy $p: legacy-probe.sql failed after the legacy boot"; return 1; }
    probe=", probe ok"
  fi
  echo "pass  legacy $p: $np renames, $nrows fixture rows kept$probe"
}

# ul_before <db>: UPGRADE-pass check of legacy-renames, run after legacy.sql and seed.sql and before the converted boot. Classifies
# each `old new` pair: `both` (old and new exist: the converted boot must leave both untouched) or `old` (only old exists: it
# must be renamed). Saves the sorted primary keys of old (and of new for `both`) and writes "k old new class" lines to
# $W/ul-$p. Fail closed: a missing old table, a table without a primary key or an old table without rows is a FAIL (return 1).
ul_before() {
  local lr=$dir/tests/boot/legacy-renames db=$1 line old new extra k=0 bad=0 class cnt
  : > "$W/ul-$p"; ul_rows=0
  [ -f "$lr" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case $line in ''|'#'*) continue;; esac
    old=; new=; extra=; read -r old new extra <<EOF
$line
EOF
    if [ -z "$old" ] || [ -z "$new" ] || [ -n "$extra" ]; then echo "FAIL  upgrade-legacy $p: legacy-renames line '$line' is not 'old new'"; bad=1; continue; fi
    k=$((k + 1))
    if [ "$(sql "$db" "SELECT to_regclass('public.$old') IS NOT NULL" 2>/dev/null)" != t ]; then echo "FAIL  upgrade-legacy $p: public.$old does not exist after legacy.sql (the fixture must be true)"; bad=1; continue; fi
    pkrows "$db" "$old" "$W/ul-old-before-$k" || { echo "FAIL  upgrade-legacy $p: $old has no readable primary key (fixtures must have PKs)"; bad=1; continue; }
    cnt=$(wc -l < "$W/ul-old-before-$k" | tr -d ' ')
    [ "$cnt" -ge 1 ] || { echo "FAIL  upgrade-legacy $p: $old has no fixture rows"; bad=1; continue; }
    class=old
    if [ "$(sql "$db" "SELECT to_regclass('public.$new') IS NOT NULL" 2>/dev/null)" = t ]; then
      class=both; pkrows "$db" "$new" "$W/ul-new-before-$k" || { echo "FAIL  upgrade-legacy $p: $new has no readable primary key (fixtures must have PKs)"; bad=1; continue; }
    fi
    ul_rows=$((ul_rows + cnt)); printf '%s %s %s %s\n' "$k" "$old" "$new" "$class" >> "$W/ul-$p"
  done < "$lr"
  [ $bad -eq 0 ]
}
# ul_after <db>: after the converted boot, per ul_before's classes: `both` keeps old with IDENTICAL keys and new with every key it had;
# `old` is gone and every old key is in new. One pass/FAIL line per plugin (nothing when the plugin has no legacy-renames).
ul_after() {
  local db=$1 k old new class bad=0 nboth=0 nren=0
  [ -s "$W/ul-$p" ] || return 0
  while read -r k old new class; do
    case $class in
      both) nboth=$((nboth + 1))
        if [ "$(sql "$db" "SELECT to_regclass('public.$old') IS NOT NULL" 2>/dev/null)" != t ]; then echo "FAIL  upgrade-legacy $p: public.$old existed beside public.$new but the converted boot removed it"; bad=1; continue; fi
        if pkrows "$db" "$old" "$W/ul-old-after-$k" && cmp -s "$W/ul-old-before-$k" "$W/ul-old-after-$k"; then :; else echo "FAIL  upgrade-legacy $p: public.$old changed (its primary keys differ after the boot)"; bad=1; fi
        if pkrows "$db" "$new" "$W/ul-new-after-$k" && [ -z "$(LC_ALL=C comm -23 "$W/ul-new-before-$k" "$W/ul-new-after-$k")" ]; then :; else echo "FAIL  upgrade-legacy $p: public.$new lost a seeded row"; bad=1; fi;;
      *) nren=$((nren + 1))
        if [ "$(sql "$db" "SELECT to_regclass('public.$old') IS NULL AND to_regclass('public.$new') IS NOT NULL" 2>/dev/null)" != t ]; then echo "FAIL  upgrade-legacy $p: public.$old must be renamed to public.$new"; bad=1; continue; fi
        if pkrows "$db" "$new" "$W/ul-new-after-$k" && [ -z "$(LC_ALL=C comm -23 "$W/ul-old-before-$k" "$W/ul-new-after-$k")" ]; then :; else echo "FAIL  upgrade-legacy $p: public.$new lost a row of $old"; bad=1; fi;;
    esac
  done < "$W/ul-$p"
  [ $bad -eq 0 ] || return 1
  echo "pass  upgrade-legacy $p: $nboth pair(s) kept untouched beside their np_ table, $nren renamed, $ul_rows fixture rows kept"
}

docker network create "$net" >/dev/null
boot_redis_start || exit 1
docker run -d --name "$pg" --network "$net" -e POSTGRES_PASSWORD=boot -e POSTGRES_DB=boot postgres:16 >/dev/null
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U postgres -d boot >/dev/null 2>&1 && break; sleep 1; done
sleep 2

for p in $(printf '%s' "$list" | tr ',' ' '); do
  dir=$root/$p; pj=$dir/plugin.json; [ -f "$pj" ] || { echo "FAIL  $p: no plugin.json under $root"; rc=1; continue; }
  schema=$(jq -r '.schema // empty' "$pj"); port=$(jq -r '.port' "$pj"); tables=$(jq -r '.tables[]?' "$pj" | tr '\n' ' ')
  # load extra-tables: optional per-plugin file listing additional tables to verify (one per line, # comments allowed)
  if [ -f "$dir/tests/boot/extra-tables" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case $line in ''|'#'*) continue;; esac
      line=$(printf '%s' "$line" | tr -d '[:space:]')
      [ -n "$line" ] && tables="$tables$line "
    done < "$dir/tests/boot/extra-tables"
  fi
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

  # ---- legacy pass
  legacy_pass || rc=1

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
  # basis-absent: manifest tables the basis build never created; they leave the before-upgrade checks ($stables) and must be true.
  stables=; absent=; absbad=0
  if [ -f "$dir/tests/boot/basis-absent" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case $line in ''|'#'*) continue;; esac
      line=$(printf '%s' "$line" | tr -d '[:space:]')
      case " $tables " in *" $line "*) ;; *) echo "FAIL  upgrade $p: basis-absent entry '$line' is not a manifest table"; absbad=1; continue;; esac
      q=$(qualified "$line"); n=$(sql "$udb" "SELECT count(*) FROM information_schema.tables WHERE table_schema = '${q%%.*}' AND table_name = '${q#*.}'" 2>/dev/null)
      [ "$n" = 0 ] || { echo "FAIL  upgrade $p: basis-absent entry '$line' exists after the basis boot (the fixture must be true)"; absbad=1; continue; }
      absent="$absent $line "
    done < "$dir/tests/boot/basis-absent"
    [ $absbad -eq 0 ] || { rc=1; continue; }
  fi
  for t in $tables; do case " ${absent:-} " in *" $t "*) ;; *) stables="$stables$t ";; esac; done
  if [ -f "$dir/tests/boot/legacy.sql" ]; then sqlfile "$udb" "$dir/tests/boot/legacy.sql" || { echo "FAIL  upgrade $p: legacy.sql failed"; rc=1; continue; }; fi
  if [ -f "$dir/tests/boot/seed.sql" ]; then sqlfile "$udb" "$dir/tests/boot/seed.sql" || { echo "FAIL  upgrade $p: seed.sql failed"; rc=1; continue; }
  else for t in $stables; do sql "$udb" "INSERT INTO $(qualified "$t") DEFAULT VALUES" >/dev/null 2>&1; done; fi
  ul_before "$udb" || { rc=1; continue; }
  before=$(counts "$udb" $stables); seeded=0; unreadable=0; empty=
  i=0; for t in $stables; do
    i=$((i + 1)); n=$(printf '%s' "$before" | awk -v k=$i '{ print $k }')
    case $n in x) unreadable=1;; 0) empty="$empty $t";; *) seeded=$((seeded + n));; esac
  done
  if [ -n "$empty" ]; then echo "FAIL  upgrade $p: no seeded row in:$empty (every table the basis creates needs at least one row)"; rc=1; continue; fi
  if [ $seeded -lt 1 ] || [ $unreadable -eq 1 ]; then echo "FAIL  upgrade $p: seeded rows before the upgrade: counts '$before' (need at least one row, every manifest table readable)"; rc=1; continue; fi
  i=0; nopk=" "; capfail=0
  for t in $stables; do
    i=$((i + 1)); pkrows "$udb" "$t" "$W/pk-before-$i"; r=$?
    case $r in 0) ;; 1) nopk="$nopk$i ";; *) capfail=1; echo "FAIL  upgrade $p: cannot read the primary keys of $t before the upgrade";; esac
  done
  [ $capfail -eq 0 ] || { rc=1; continue; }
  boot "nself-tu-c-$pid" "$tag" "$udb" "$port"
  printf '%s\n' "$LOGS" | grep 'legacy table' | while IFS= read -r line; do echo "note  upgrade $p: log: $line"; done
  ul_after "$udb" || rc=1
  after=$(counts "$udb" $stables); lost=0; i=0
  for t in $stables; do
    i=$((i + 1)); n=$(printf '%s' "$before" | awk -v k=$i '{ print $k }'); a=$(printf '%s' "$after" | awk -v k=$i '{ print $k }')
    case $nopk in
      *" $i "*) case $a in x|'') lost=1;; *) [ "$a" -ge "$n" ] || { lost=1; echo "FAIL  upgrade $p: table $t lost rows (count $n -> $a)"; };; esac;;
      *) if pkrows "$udb" "$t" "$W/pk-after-$i" && [ -z "$(LC_ALL=C comm -23 "$W/pk-before-$i" "$W/pk-after-$i")" ]; then :; else lost=1; echo "FAIL  upgrade $p: table $t lost a seeded row or changed its primary key (missing keys: $(LC_ALL=C comm -23 "$W/pk-before-$i" "$W/pk-after-$i" 2>/dev/null | head -3 | tr '\n' ' '))"; fi;;
    esac
  done
  if mig_ok "$expected" && tables_present "$udb" $tables && [ $lost -eq 0 ]; then echo "pass  upgrade $p: basis seeded $seeded row(s) (counts $before), converted boot applied=expected=$expected, counts after $after"
  else echo "FAIL  upgrade $p: want migrations $expected/$expected, tables present, no row lost; health='$H' counts before '$before' after '$after'"; printf '%s\n' "$LOGS" | tail -8; rc=1; fi
  docker rmi -f "$tag" "$btag" >/dev/null 2>&1
done
[ $rc -eq 0 ] && echo "tables-upgrade: ok" || echo "tables-upgrade: FAIL"; exit $rc
