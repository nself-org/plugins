#!/usr/bin/env bash
# a05.sh: A5 a dirty working tree runs on a real node: the node result names the snapshot, the
# untracked secret-shaped file stays home, and the private ref is gone afterwards.
# Needs CI_CONF_NODE1. Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A5
conf_need CI_CONF_NODE1
conf_bin
d="$CONF_DIR/dirty"
conf_sample go "$d" || conf_fail "sample could not be written"
printf 'package sample\n\nfunc Two() int { return 2 }\n' > "$d/two.go"
printf 'placeholder\n' > "$d/.env"
(cd "$d" && conf_timeout 600 "$BIN" --where "$CI_CONF_NODE1" --check --no-status --json) >>"$CONF_EVID" 2>&1 || conf_fail "dirty-tree run on $CI_CONF_NODE1 failed"
git -C "$d" for-each-ref | grep -q 'refs/nself-ci/offload' && conf_fail "a private offload ref was left behind"
grep -q 'placeholder' "$CONF_EVID" && conf_fail "the secret-shaped file reached the node output"
conf_pass "dirty tree ran on the node; ref released; secret file stayed home"
