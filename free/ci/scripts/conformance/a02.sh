#!/usr/bin/env bash
# a02.sh: A2 the cli unit suite with TMPDIR on an external volume whose root is 0775 finishes in
# under 10 minutes. The code fix is P7-HYG-49; this leg only re-proves it.
# Needs CI_CONF_EXTVOL (the volume root) and CI_CONF_CLI_DIR (a cli checkout). Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A2
conf_need CI_CONF_EXTVOL CI_CONF_CLI_DIR
[ -d "$CI_CONF_EXTVOL" ] || conf_skip "missing host: $CI_CONF_EXTVOL is not mounted"
mode=$(stat -f '%Lp' "$CI_CONF_EXTVOL" 2>/dev/null || stat -c '%a' "$CI_CONF_EXTVOL")
[ "$mode" = "775" ] || conf_skip "missing host: $CI_CONF_EXTVOL root is $mode, not 775"
tmp=$(mktemp -d "$CI_CONF_EXTVOL/ci-conf-a2.XXXXXX") || conf_fail "cannot create a temp folder on the volume"
trap 'rm -rf "$tmp"' EXIT
conf_timeout 600 env TMPDIR="$tmp" go -C "$CI_CONF_CLI_DIR" test -count=1 ./... >>"$CONF_EVID" 2>&1
case $? in
  0) conf_pass "cli unit suite passed with TMPDIR on the 0775 volume" ;;
  124) conf_fail "cli unit suite took longer than 10 minutes" ;;
  *) conf_fail "cli unit suite failed" ;;
esac
