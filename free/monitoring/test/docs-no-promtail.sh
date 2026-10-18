#!/usr/bin/env bash
# docs-no-promtail.sh: the docs describe Grafana Alloy, not the retired Promtail (D-0936, follows P7-PLUG-58).
# Checks the root README, the Monitoring wiki page and the Deployment guide for "promtail" (any case) and for the
# old Promtail port 9080, and that the README services table lists alloy on its real port (12345) at a path that exists.
# Exit: 0 clean, 1 a stale mention (named), 2 a missing input.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
docs="README.md .github/wiki/plugins/Monitoring.md .github/wiki/guides/Deployment.md"
rc=0
for f in $docs; do
  [ -f "$root/$f" ] || { echo "docs-no-promtail: missing $f" >&2; exit 2; }
  if hits=$(grep -niE 'promtail|\b9080\b' "$root/$f"); then
    printf '%s\n' "$hits" | sed "s|^|FAIL $f:|"; rc=1
  else
    echo "ok   $f has no Promtail or port 9080"
  fi
done
row=$(grep -E '^\| \[alloy\]\(' "$root/README.md" || true)
if [ -z "$row" ]; then echo "FAIL README.md: no alloy row in the services table"; rc=1
else
  path=$(printf '%s' "$row" | sed -n 's/^| \[alloy\](\.\/\([^)]*\)).*/\1/p')
  printf '%s' "$row" | grep -q '12345' || { echo "FAIL README.md: alloy row does not give port 12345"; rc=1; }
  [ -n "$path" ] && [ -e "$root/$path" ] || { echo "FAIL README.md: alloy row path '${path:-?}' does not exist"; rc=1; }
fi
[ $rc -eq 0 ] && echo "docs-no-promtail: docs describe Alloy"
exit $rc
