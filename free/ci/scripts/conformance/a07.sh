#!/usr/bin/env bash
# a07.sh: A7 race across two real nodes: `--where all` finishes with exactly one result and the
# slower node ends with no job process left. Needs CI_CONF_NODE1 and CI_CONF_NODE2. Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A7
conf_need CI_CONF_NODE1 CI_CONF_NODE2
conf_bin
d="$CONF_DIR/race"
conf_sample go "$d" || conf_fail "sample could not be written"
out=$(cd "$d" && conf_timeout 600 "$BIN" --where all --check --no-status --json 2>&1) || conf_fail "race run failed"
printf '%s\n' "$out" >>"$CONF_EVID"
n=$(printf '%s' "$out" | grep -c '"winner"')
[ "$n" = "1" ] || conf_fail "race recorded $n winners, want 1"
printf '%s' "$out" | grep -q 'cancelled' || conf_fail "no sibling was cancelled"
conf_pass "one winner, the other sibling cancelled"
