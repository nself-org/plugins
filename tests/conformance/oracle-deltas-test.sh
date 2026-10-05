#!/usr/bin/env bash
# oracle-deltas-test.sh: negative proofs for the oracle delta mechanism (no Docker, no network).
# Each case builds a fake converted output and requires apply_deltas to mask only what its row names.
set -u
HERE=$(cd "$(dirname "$0")" && pwd); . "$HERE/../../scripts/conformance/lib.sh"; . "$CONF_DIR/oracle/deltas.sh"
W=$(conf_work)/deltas-test; rm -rf "$W"; mkdir -p "$W"; rc=0
LINE_B='PLUGIN_BETA_INTERNAL_URL=http://plugin-beta:3902'
row() { printf 'deltas:\n  - slug: %s\n    file: %s\n    key: "%s"\n    from: "%s"\n    to: "%s"\n    owner_ref: P7-TEST\n    expires: %s\n' "$@" > "$W/d.yaml"; }
expect() { # name, want rc, command... ; prints pass/FAIL
  local name=$1 want=$2; shift 2; "$@" >"$W/o.txt" 2>&1; local got=$?
  if [ "$got" -eq "$want" ]; then echo "pass  deltas: $name"; else echo "FAIL  deltas: $name (rc $got, want $want)"; sed 's/^/        /' "$W/o.txt"; rc=1; fi
}
mk() { rm -rf "$W/out"; mkdir -p "$W/out"; printf '%s\n' "$LINE_B" 'PLUGIN_GAMMA_INTERNAL_URL=http://plugin-gamma:3903' > "$W/out/compose-env-urls.txt"; printf '%s\n' "$LINE_B" > "$W/out/seed.sql"; }
still() { grep -qxF "$2" "$W/out/$1"; }

mk; row beta compose-env-urls.txt "$LINE_B" - "http://plugin-beta:3902" 2026-12-31
expect "exact row applies" 0 apply_deltas "$W/out" "beta gamma" "$W/d.yaml"
still compose-env-urls.txt "$LINE_B" && { echo "FAIL  deltas: exact line was not removed"; rc=1; } || echo "pass  deltas: exact line removed"
still compose-env-urls.txt 'PLUGIN_GAMMA_INTERNAL_URL=http://plugin-gamma:3903' && echo "pass  deltas: other slug's line is not masked" || { echo "FAIL  deltas: other slug's line was masked"; rc=1; }
still seed.sql "$LINE_B" && echo "pass  deltas: same line in another file is not masked" || { echo "FAIL  deltas: another file was masked"; rc=1; }

mk; printf '%s\n' "$LINE_B" 'PLUGIN_BETA_INTERNAL_URL=http://evil:1' > "$W/out/compose-env-urls.txt"; row beta compose-env-urls.txt "$LINE_B" - x 2026-12-31
apply_deltas "$W/out" "beta" "$W/d.yaml" >/dev/null
still compose-env-urls.txt 'PLUGIN_BETA_INTERNAL_URL=http://evil:1' && echo "pass  deltas: a regression line containing the key is not masked" || { echo "FAIL  deltas: regression line was masked"; rc=1; }

mk; printf 'PLUGIN_BETA_INTERNAL_URL=http://plugin-beta:3902 extra\n' > "$W/out/compose-env-urls.txt"; row beta compose-env-urls.txt "$LINE_B" - x 2026-12-31
expect "a row that matches only a longer line is stale" 1 apply_deltas "$W/out" "beta" "$W/d.yaml"
mk; row beta compose-env-urls.txt "PLUGIN_BETA_INTERNAL_URL=http://nowhere:1" - x 2026-12-31
expect "a stale row fails" 1 apply_deltas "$W/out" "beta" "$W/d.yaml"
mk; row beta nosuchfile.txt "$LINE_B" - x 2026-12-31
expect "a row naming a missing file fails" 1 apply_deltas "$W/out" "beta" "$W/d.yaml"
mk; row beta compose-env-urls.txt "$LINE_B" - x 2099-12-31
expect "an expiry after the cap (2099) fails" 1 apply_deltas "$W/out" "beta" "$W/d.yaml"
mk; row beta compose-env-urls.txt "$LINE_B" - x 2020-01-01
expect "an expired row fails" 1 apply_deltas "$W/out" "beta" "$W/d.yaml"
mk; row other compose-env-urls.txt "$LINE_B" - x 2026-12-31
expect "a row for a slug not under test is skipped, not stale" 0 apply_deltas "$W/out" "beta" "$W/d.yaml"
still compose-env-urls.txt "$LINE_B" && echo "pass  deltas: a skipped row masks nothing" || { echo "FAIL  deltas: a skipped row masked a line"; rc=1; }
# replacement rows: literal (not regex) key and per-file
printf 'port 3855 nself-cloud\nport 3855 other\n' > "$W/out/docker-compose.yml"; printf 'port 3855 nself-cloud\n' > "$W/out/list.txt"
row nself-cloud docker-compose.yml "nself-cloud" 3845 3855 2026-12-31
expect "replacement row applies" 0 apply_deltas "$W/out" "nself-cloud" "$W/d.yaml"
still docker-compose.yml 'port 3845 nself-cloud' && still docker-compose.yml 'port 3855 other' && still list.txt 'port 3855 nself-cloud' && echo "pass  deltas: replacement touches only matching lines of its own file" || { echo "FAIL  deltas: replacement scope wrong"; rc=1; }
printf 'axb\n' > "$W/out/docker-compose.yml"; row nself-cloud docker-compose.yml "a.b" 1 x 2026-12-31
expect "a regex key does not match literally-different text" 1 apply_deltas "$W/out" "nself-cloud" "$W/d.yaml"
[ $rc -eq 0 ] && echo "deltas: ok" || echo "deltas: FAIL"; exit $rc
