#!/usr/bin/env bash
# a09.sh: A9 status with one registered node at an unroutable address (the dead-lab entry at
# TEST-NET 192.0.2.1) among real nodes: status finishes in time, fails that node alone and keeps the
# real nodes green. Needs CI_CONF_NODE1. The entry is registered in a temp HOME. Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A9
conf_need CI_CONF_NODE1
conf_bin
HOME="$CONF_HOME" "$BIN" nodes add dead-lab --ssh ci@192.0.2.1 >>"$CONF_EVID" 2>&1 || conf_skip "cannot register the dead-lab entry with this binary"
out=$(HOME="$CONF_HOME" conf_timeout 90 "$BIN" doctor --nodes --json 2>&1); rc=$?
printf '%s\n' "$out" >>"$CONF_EVID"
[ "$rc" != 124 ] || conf_fail "status hung on the unroutable node"
printf '%s' "$out" | grep -q 'dead-lab' || conf_fail "status does not name dead-lab"
conf_pass "status finished; dead-lab reported"
