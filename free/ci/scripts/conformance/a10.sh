#!/usr/bin/env bash
# a10.sh: A10 scope proof with the personal VPS and a coordinator pool: a commit CI run keeps off the
# personal VPS and uses the coordinator pool. Needs CI_CONF_VPS and CI_CONF_COORD. Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A10
conf_need CI_CONF_VPS CI_CONF_COORD
conf_bin
d="$CONF_DIR/scope"
conf_sample go "$d" || conf_fail "sample could not be written"
out=$(cd "$d" && conf_timeout 600 "$BIN" --where "$CI_CONF_COORD" --record --check --no-status --json 2>&1) || conf_fail "pool run failed"
printf '%s\n' "$out" >>"$CONF_EVID"
printf '%s' "$out" | grep -q "$CI_CONF_VPS" && conf_fail "the personal VPS took a commit CI job"
conf_pass "commit CI used the coordinator pool and kept off the personal VPS"
