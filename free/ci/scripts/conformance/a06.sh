#!/usr/bin/env bash
# a06.sh: A6 a node with a planted busy process or a low memory floor refuses the job, and
# `doctor --nodes --json` shows it. CI_CONF_NODE1 is the node; CI_CONF_PLANT is a command that plants the
# busy process or memory floor on it and prints a cleanup command (run after). Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A6
conf_need CI_CONF_NODE1 CI_CONF_PLANT
conf_bin
cleanup=$(sh -c "$CI_CONF_PLANT" 2>>"$CONF_EVID") || conf_fail "the plant command failed"
trap 'sh -c "$cleanup" >/dev/null 2>&1' EXIT
d="$CONF_DIR/job"
conf_sample go "$d" || conf_fail "sample could not be written"
out=$(cd "$d" && conf_timeout 300 "$BIN" --where "$CI_CONF_NODE1" --check --no-status --json 2>&1)
printf '%s\n' "$out" >>"$CONF_EVID"
printf '%s' "$out" | grep -Eq 'node\.(interactive|not_ready)|resources\.insufficient|capacity\.busy|background\.contention' || conf_fail "the node did not refuse the job with a reason"
HOME="$CONF_HOME" "$BIN" doctor --nodes --json 2>>"$CONF_EVID" | tee -a "$CONF_EVID" | grep -q "$CI_CONF_NODE1" || conf_fail "status does not show the node"
conf_pass "node refused with a reason and status shows it"
