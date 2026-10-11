#!/usr/bin/env bash
# Purpose: the day2 leg of tests/kind/run.sh (P7-DEPL-08, EPIC D8): seed three tables with 1,000
#   rows each, `nself k8s backup --json`, delete the namespace, install again, check that
#   `restore` without --yes exits 4, then `restore --yes --json` and an independent check.
# Inputs: the run.sh environment: the nself function, $project, $KUBECONFIG, step, install_stack
#   (stack.sh) and jq on PATH. The kubeconfig context's namespace is nself-day2.
# Outputs: run_day2 returns 0 when every check passed; a failed check prints FAIL and returns 1.
# Constraints: only `nself k8s ...` installs, backs up and restores; kubectl is used to seed,
#   delete the namespace and to read the data back independently (psql inside the postgres pod,
#   SQL on stdin, the password read from the pod's own environment). No secret is on an argv.
#   Synthetic rows only.

# shellcheck disable=SC2154

# fail and envelope_ok come from contract.sh.
# shellcheck disable=SC1091
. "$here/contract.sh"

# pg_sql: run the SQL on stdin in the postgres pod, print bare values.
pg_sql() {
  # The single quotes are deliberate: the variables expand in the pod, not here.
  # shellcheck disable=SC2016
  kubectl exec -i postgres-0 -c postgres -- sh -c \
    'export PGPASSWORD="$POSTGRES_PASSWORD"; U="${POSTGRES_USER:-postgres}"; exec psql -X -q -A -t -v ON_ERROR_STOP=1 -U "$U" -d "${POSTGRES_DB:-$U}"'
}

# content_digest: one md5 over every row of the three seed tables, taken with the pod's psql.
content_digest() {
  pg_sql <<'SQL'
SELECT md5((SELECT string_agg(t::text, ',' ORDER BY id) FROM public.d2_a t) ||
           (SELECT string_agg(t::text, ',' ORDER BY id) FROM public.d2_b t) ||
           (SELECT string_agg(t::text, ',' ORDER BY id) FROM public."d2 ""odd""" t));
SQL
}

seed_day2() {
  pg_sql <<'SQL'
CREATE TABLE public.d2_a (id int PRIMARY KEY, note text, at timestamptz);
CREATE TABLE public.d2_b (id int PRIMARY KEY, doc jsonb, val double precision);
CREATE TABLE public."d2 ""odd""" (id int PRIMARY KEY, raw bytea);
INSERT INTO public.d2_a SELECT g, 'row ' || g, '2026-10-11 00:00:00+00'::timestamptz + g * interval '1 second' FROM generate_series(1, 1000) g;
INSERT INTO public.d2_b SELECT g, jsonb_build_object('n', g, 'k', 'v' || g), g / 7.0 FROM generate_series(1, 1000) g;
INSERT INTO public."d2 ""odd""" SELECT g, decode(md5(g::text), 'hex') FROM generate_series(1, 1000) g;
SQL
}

# user_table_count: ordinary tables outside the system schemas, as the database counts them.
user_table_count() {
  pg_sql <<'SQL'
SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp\_%';
SQL
}

# check_backup_envelope <json>: a data envelope that names the dump and the count of every table.
check_backup_envelope() {
  local out="$1" file n_env n_db
  envelope_ok "$out" '.command == "k8s backup" and (has("error") | not)' || fail "backup --json is not a data envelope"
  file="$(printf '%s' "$out" | jq -r '.data.file')"
  printf '%s' "$file" | grep -Eq '/nself-kind_full_[0-9]{8}_[0-9]{6}\.dump$' || fail "dump name is not in the core layout: $file"
  [ -s "$file" ] || fail "dump file is missing or empty: $file"
  [ "$(printf '%s' "$out" | jq -r '.data.metadata_file')" = "$(dirname "$file")/.meta/$(basename "$file").json" ] || fail "sidecar is not <dir>/.meta/<file>.json"
  [ -s "$(dirname "$file")/.meta/$(basename "$file").json" ] || fail "sidecar file is missing"
  for t in d2_a d2_b 'd2 "odd"'; do
    printf '%s' "$out" | jq -e --arg t "$t" '.data.tables[] | select(.schema == "public" and .table == $t and .rows == 1000)' >/dev/null ||
      fail "the envelope does not give 1000 rows for public.$t"
  done
  n_env="$(printf '%s' "$out" | jq '.data.tables | length')"
  n_db="$(user_table_count)"
  [ "$n_env" = "$n_db" ] || fail "the envelope lists $n_env tables, the database has $n_db"
  [ "$(printf '%s' "$out" | jq '[.data.tables[] | select(.schema == "auth" or .schema == "storage")] | length')" -gt 0 ] ||
    fail "no auth or storage table is fingerprinted"
}

run_day2() {
  local out rc rc_json dump digest i=0
  step "seed three tables with 1000 rows each"
  seed_day2
  digest="$(content_digest)"
  echo "content digest before: $digest"

  step "nself k8s backup --json"
  out="$(cd "$project" && nself k8s backup --json --cluster "$KUBECONFIG")"
  printf '%s\n' "$out" | jq -c '{command, file: .data.file, bytes: .data.bytes, tables: (.data.tables | length)}'
  check_backup_envelope "$out"
  dump="$(printf '%s' "$out" | jq -r '.data.file')"
  [ "$(stat -c %a "$dump" 2>/dev/null || stat -f %Lp "$dump")" = "600" ] || fail "dump mode is not 0600"

  step "delete the namespace and wait until it is gone"
  kubectl delete namespace nself-day2 --wait=true --timeout=300s
  while kubectl get namespace nself-day2 >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -lt 90 ] || fail "namespace nself-day2 is still there"
    sleep 2
  done
  kubectl create namespace nself-day2

  step "install again into the empty namespace"
  install_stack
  [ "$(pg_sql <<<"SELECT count(*) FROM pg_tables WHERE tablename = 'd2_a';")" = "0" ] || fail "the new database already has d2_a: not a fresh install"

  step "restore without --yes exits 4 and changes nothing"
  set +e
  (cd "$project" && nself k8s restore "$dump" --cluster "$KUBECONFIG" </dev/null >/dev/null 2>&1)
  rc=$?
  out="$(cd "$project" && nself k8s restore "$dump" --json --cluster "$KUBECONFIG" </dev/null 2>/dev/null)"
  rc_json=$?
  set -e
  [ "$rc" -eq 4 ] && [ "$rc_json" -eq 4 ] || fail "restore without --yes exited $rc / $rc_json, want 4 / 4"
  envelope_ok "$out" '.command == "k8s restore" and .error.code == "E403"' || fail "the refusal is not an E403 envelope"
  [ "$(pg_sql <<<"SELECT count(*) FROM pg_tables WHERE tablename = 'd2_a';")" = "0" ] || fail "the refused restore changed the database"

  step "nself k8s restore --yes --json"
  out="$(cd "$project" && nself k8s restore "$dump" --yes --json --cluster "$KUBECONFIG")"
  printf '%s\n' "$out" | jq -c '{command, verified: .data.verified, tables: (.data.tables | length)}'
  envelope_ok "$out" '.command == "k8s restore" and (has("error") | not) and .data.verified == true' || fail "restore --yes --json did not verify"
  for t in d2_a d2_b 'd2 "odd"'; do
    printf '%s' "$out" | jq -e --arg t "$t" '.data.tables[] | select(.table == $t and .rows == 1000)' >/dev/null || fail "restore reports no 1000 rows for $t"
  done

  step "independent check: counts and content digest through the pod's psql"
  [ "$(pg_sql <<<"SELECT (SELECT count(*) FROM public.d2_a) || '/' || (SELECT count(*) FROM public.d2_b) || '/' || (SELECT count(*) FROM public.\"d2 \"\"odd\"\"\");")" = "1000/1000/1000" ] ||
    fail "row counts after the restore are not 1000/1000/1000"
  [ "$(content_digest)" = "$digest" ] || fail "content digest after the restore differs from before the backup"
  echo "content digest after: equal"
}
