#!/usr/bin/env bash
# nsentry-report-writer: snapshot nSentry issues into timestamped MD reports.
# Idempotent per issue-key (won't rewrite the same open issue). Sources:
#   - components not operational   - SSL certs expiring < warn days
#   - recent ingested errors       - pushed CI failures (via --ci arg file)
set -euo pipefail
ERR_DIR=/opt/nself-ops/errors
SEEN=/opt/nself-ops/errors/.seen
touch "$SEEN"
PSQL(){ docker exec -i ops-postgres psql -U nself -d nself -tAc "$1" 2>/dev/null; }
ts(){ date -u +%Y%m%d-%H%M%S; }
emit(){ # key, slug, title, severity, body
  local key="$1" slug="$2" title="$3" sev="$4" body="$5"
  grep -qxF "$key" "$SEEN" && return 0
  local h=$(printf "%s" "$key"|md5sum|cut -c1-6); local f="$ERR_DIR/$(ts)-$h-$slug.md"
  { echo "---"; echo "id: $key"; echo "created_at: $(date -u +%FT%TZ)";
    echo "title: \"$title\""; echo "severity: $sev"; echo "source: nsentry";
    echo "---"; echo; echo "# $title"; echo; echo "$body"; } > "$f"
  echo "$key" >> "$SEEN"; echo "WROTE $f"
}
# 1) non-operational components
PSQL "SELECT name||'|'||status FROM np_status.np_status_components WHERE status<>'operational'" | while IFS='|' read -r n s; do
  [ -z "$n" ] && continue
  emit "comp:$n:$s" "down-${n//./-}" "$n is $s" "high" "Component **$n** reported status **$s** by nSentry uptime monitoring. Check the service and recent deploys."
done
# 2) SSL expiring < 30d
PSQL "SELECT target_id||'|'||days_remaining FROM np_uptime_tls WHERE days_remaining < 30" | while IFS='|' read -r t d; do
  [ -z "$t" ] && continue
  emit "ssl:$t:$d" "ssl-expiry" "SSL cert expiring in ${d}d (target $t)" "medium" "TLS certificate for target \`$t\` expires in **${d} days**. Renew before expiry."
done
# 3) pushed CI failure (optional arg: path to a json/text payload)
if [ "${1:-}" = "--ci" ] && [ -f "${2:-}" ]; then
  body=$(cat "$2"); key="ci:$(echo "$body"|md5sum|cut -c1-12)"
  emit "$key" "ci-failure" "CI failure" "high" "$body"
fi
echo "report-writer done; total reports: $(ls -1 $ERR_DIR/*.md 2>/dev/null|wc -l|tr -d ' ')"
