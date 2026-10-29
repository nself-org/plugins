#!/usr/bin/env bash
# a01.sh: A1 gate tables are identical on macOS arm64 and Linux amd64 for Go, Node and Go+Node
# sample repos; no system change, no dialog, no hang (every gate run has a 600 s deadline).
# Needs CI_CONF_MAC=local and CI_CONF_LINUX. Exit 0 pass, 1 fail, 3 skip.
. "$(dirname "$0")/lib.sh"
conf_begin A1
conf_need CI_CONF_MAC CI_CONF_LINUX
conf_bin
before=$(ls -A "$HOME" | shasum | cut -d' ' -f1)
for kind in go node gonode; do
  conf_sample "$kind" "$CONF_DIR/$kind" || conf_fail "sample $kind could not be written"
  a=$(conf_table local "$CONF_DIR/$kind")
  b=$(conf_table "$CI_CONF_LINUX" "$CONF_DIR/$kind")
  [ -n "$a" ] && [ -n "$b" ] || conf_fail "$kind: an empty gate table (hang or no output)"
  [ "$a" = "$b" ] || { printf '%s\n---\n%s\n' "$a" "$b" >>"$CONF_EVID"; conf_fail "$kind: gate tables differ between hosts"; }
done
after=$(ls -A "$HOME" | shasum | cut -d' ' -f1)
[ "$before" = "$after" ] || conf_fail "the home folder changed during the run"
conf_pass "gate tables identical for go, node and gonode"
