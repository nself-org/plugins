#!/usr/bin/env bash
# a04.sh: A4 the hosted-tier observer reads recent commits of an existing public repository with a
# workflow (read only; no repository is created), plus the billing-blocked case from fixtures.
# Needs CI_CONF_MAC=local and CI_CONF_HOSTED_REPO (default nself-org/plugins) readable through gh. Exit 0, 1 or 3.
. "$(dirname "$0")/lib.sh"
conf_begin A4
conf_need CI_CONF_MAC
conf_bin
command -v gh >/dev/null 2>&1 || conf_skip "missing tool: gh"
repo=${CI_CONF_HOSTED_REPO:-nself-org/plugins}
shas=$(gh api "repos/$repo/commits?per_page=3" --jq '.[].sha' 2>>"$CONF_EVID") || conf_skip "missing access: $repo is not readable"
for s in $shas; do
  out=$(conf_timeout 120 "$BIN" --where hosted --explain --sha "$s" --repo "$repo" --json 2>>"$CONF_EVID") || conf_fail "observer failed on $s"
  printf '%s\n' "$out" >>"$CONF_EVID"
  printf '%s' "$out" | grep -Eq 'hosted\.(success|failure|queued|running|absent|cancelled)' || conf_fail "no hosted decision for $s"
done
fix=$(cd "$(dirname "$0")/../.." && CGO_ENABLED=0 go test -mod=vendor -count=1 -run '^TestConformanceA4Hosted$' ./test/sim/auto/ 2>&1) || conf_fail "billing-blocked fixture failed"
printf '%s\n' "$fix" >>"$CONF_EVID"
conf_pass "observer decided recent commits read only; billing-blocked fixture passed"
