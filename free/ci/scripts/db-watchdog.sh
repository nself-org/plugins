#!/usr/bin/env bash
# db-watchdog.sh — sentry-box DB health check + auto-restart
#
# Purpose:
#   Checks postgres and redis containers every invocation (designed to be called
#   by cron every 2 minutes — cron IS the loop, this is single-shot). On failure:
#   emits a deduped MD alert to REPORT_DIR (picked up by Claude inbox sync) and
#   restarts the affected container.
#
# Usage (cron entry):
#   */2 * * * * /opt/nself-ops/bin/db-watchdog.sh >>/opt/nself-ops/errors/.db-watchdog.log 2>&1
#
# Env vars (all have defaults):
#   REPORT_DIR          — where to write MD alerts  (default: /opt/nself-ops/errors)
#   POSTGRES_CONTAINER  — docker container name      (default: ops-postgres)
#   REDIS_CONTAINER     — docker container name      (default: ops-redis)
#
# Dedup: one alert per service per 10 minutes via /tmp lockfile.
# DB-independent: never calls psql/pg_isready on the host; always via docker exec.
#
# Redis probe is AUTH-AWARE: it uses the container's own REDIS_PASSWORD env if
# set, and treats a NOAUTH/WRONGPASS reply as ALIVE (the server answered — it is
# up). Restart only fires on connection-refused / timeout / no-response.
# History: an unauthenticated `redis-cli ping` against a requirepass redis
# returns NOAUTH; the old check judged that "down" and restart-looped a healthy
# redis every 5 minutes for 6+ hours (cam-sentry incident).

set -euo pipefail

REPORT_DIR="${REPORT_DIR:-/opt/nself-ops/errors}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-ops-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-ops-redis}"
DEDUP_TTL=600  # 10 minutes in seconds

mkdir -p "$REPORT_DIR"

ts_iso()  { date -u +%FT%TZ; }
ts_file() { date -u +%Y%m%d-%H%M%S; }
hash6()   { printf "%s" "$1" | md5sum | cut -c1-6; }

# emit <service> <title> <severity> <body>
emit() {
  local svc="$1" title="$2" sev="$3" body="$4"
  local lock="/tmp/db-watchdog-${svc}.lock"

  # dedup: skip if lock file exists and is less than DEDUP_TTL seconds old
  if [ -f "$lock" ]; then
    local age=$(( $(date +%s) - $(date -r "$lock" +%s 2>/dev/null || echo 0) ))
    if [ "$age" -lt "$DEDUP_TTL" ]; then
      echo "[db-watchdog] dedup skip: $svc alert already fired ${age}s ago" >&2
      return 0
    fi
  fi

  local key="db-watchdog:${svc}:$(ts_file)"
  local h; h=$(hash6 "$key")
  local f="${REPORT_DIR}/$(ts_file)-${h}-db-watchdog-${svc}.md"

  {
    echo "---"
    echo "id: ${key}"
    echo "created_at: $(ts_iso)"
    echo "title: \"${title}\""
    echo "severity: ${sev}"
    echo "source: db-watchdog"
    echo "---"
    echo
    echo "# ${title}"
    echo
    echo "${body}"
    echo
    echo "**Container:** \`${svc}\`  "
    echo "**Time:** $(ts_iso)  "
    echo "**Action:** container restart issued automatically."
  } > "$f"

  touch "$lock"
  echo "[db-watchdog] alert written: $f" >&2
}

check_postgres() {
  local ok
  ok=$(docker exec "$POSTGRES_CONTAINER" pg_isready -U postgres 2>&1) || true
  if echo "$ok" | grep -q "accepting connections"; then
    echo "[db-watchdog] postgres OK"
  else
    echo "[db-watchdog] postgres FAILED: $ok" >&2
    emit "$POSTGRES_CONTAINER" \
      "Postgres container unhealthy on sentry box" \
      "critical" \
      "Container \`${POSTGRES_CONTAINER}\` failed \`pg_isready\` check.

\`\`\`
${ok}
\`\`\`

Automatic restart was issued. Check disk space, OOM, and docker logs:
\`\`\`bash
docker logs --tail 100 ${POSTGRES_CONTAINER}
\`\`\`"
    docker restart "$POSTGRES_CONTAINER" || true
    echo "[db-watchdog] restarted $POSTGRES_CONTAINER" >&2
  fi
}

check_redis() {
  local ok
  # Auth-aware probe: use the container's own REDIS_PASSWORD (requirepass
  # deployments) via REDISCLI_AUTH — no -a plaintext-warning noise. The env var
  # must expand INSIDE the container, hence the single-quoted sh -c.
  ok=$(docker exec "$REDIS_CONTAINER" sh -c \
        'if [ -n "${REDIS_PASSWORD:-}" ]; then REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli ping; else redis-cli ping; fi' \
        2>&1) || true
  if echo "$ok" | grep -qi "pong"; then
    echo "[db-watchdog] redis OK"
    return 0
  fi
  # An auth error IS a reply: the server is up and answering. NEVER restart on
  # auth errors — only on connection-refused/timeout/no-response.
  if echo "$ok" | grep -qiE "NOAUTH|WRONGPASS|ERR AUTH|invalid password"; then
    echo "[db-watchdog] redis OK (alive — replied with auth error; probe lacks credentials, NOT restarting)"
    return 0
  fi
  echo "[db-watchdog] redis FAILED: $ok" >&2
  emit "$REDIS_CONTAINER" \
    "Redis container unhealthy on sentry box" \
    "high" \
    "Container \`${REDIS_CONTAINER}\` did not respond PONG to \`redis-cli ping\`.

\`\`\`
${ok}
\`\`\`

Automatic restart was issued. Check logs:
\`\`\`bash
docker logs --tail 100 ${REDIS_CONTAINER}
\`\`\`"
  docker restart "$REDIS_CONTAINER" || true
  echo "[db-watchdog] restarted $REDIS_CONTAINER" >&2
}

check_postgres
check_redis
echo "[db-watchdog] done at $(ts_iso)"
