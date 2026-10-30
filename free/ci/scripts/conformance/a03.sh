#!/usr/bin/env bash
# a03.sh: A3 three real hosts each run `ci serve watch` on a private scratch repository; every pushed
# commit gets exactly one final status. Needs CI_CONF_NODE1, CI_CONF_NODE2, CI_CONF_MAC and a scratch
# repository the hosts can reach in CI_CONF_WATCH_REPO (owner/name). Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A3
conf_need CI_CONF_MAC CI_CONF_NODE1 CI_CONF_NODE2 CI_CONF_WATCH_REPO
conf_bin
sha=$(git ls-remote "https://github.com/$CI_CONF_WATCH_REPO" HEAD 2>>"$CONF_EVID" | cut -f1)
[ -n "$sha" ] || conf_fail "the scratch repository has no head"
for h in local "$CI_CONF_NODE1" "$CI_CONF_NODE2"; do
  conf_timeout 900 "$BIN" --where "$h" serve watch --once --repo "$CI_CONF_WATCH_REPO" >>"$CONF_EVID" 2>&1 || conf_fail "watch pass failed on $h"
done
n=$(gh api "repos/$CI_CONF_WATCH_REPO/commits/$sha/statuses" --jq '[.[]|select(.context=="nself-ci" and (.state=="success" or .state=="failure"))]|length' 2>>"$CONF_EVID")
[ "$n" = "1" ] || conf_fail "commit has $n final statuses, want 1"
conf_pass "exactly one final status across three hosts"
