#!/usr/bin/env bash
# a08.sh: A8 selftest and status flag a planted stuck watcher, a leaked temp folder and an infra
# failure (needs CI_CONF_MAC=local). All three are planted inside the temp HOME of this leg. Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A8
conf_need CI_CONF_MAC
conf_bin
mkdir -p "$CONF_HOME/.nself/ci/tmp/leaked-run" "$CONF_HOME/.nself/ci/watch"
touch -t 202001010000 "$CONF_HOME/.nself/ci/tmp/leaked-run"
printf '999999 stuck\n' > "$CONF_HOME/.nself/ci/watch/watcher.pid"
touch -t 202001010000 "$CONF_HOME/.nself/ci/watch/watcher.pid"
out=$(HOME="$CONF_HOME" CI_CONF_PLANT_INFRA=1 conf_timeout 300 "$BIN" doctor --json 2>&1)
printf '%s\n' "$out" >>"$CONF_EVID"
printf '%s' "$out" | grep -qi 'watch' || conf_fail "the stuck watcher was not flagged"
printf '%s' "$out" | grep -Eqi 'leak|stale|temp' || conf_fail "the leaked temp folder was not flagged"
st=$(HOME="$CONF_HOME" conf_timeout 120 "$BIN" doctor --nodes --json 2>&1); printf '%s\n' "$st" >>"$CONF_EVID"
printf '%s' "$st" | grep -Eqi 'fail|warn|infra' || conf_fail "status shows no infra failure"
conf_pass "stuck watcher, leaked temp folder and infra failure flagged"
