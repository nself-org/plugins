#!/usr/bin/env bash
# disk-guard.sh — pre-emptive disk alert + deep reclaim on sentry box
#
# Purpose:
#   Runs every 5 minutes (cron IS the loop, this is single-shot). Checks disk
#   usage on /. At >DISK_WARN_PCT: reclaims proactively (deep_reclaim) and
#   emits a deduped MD alert (max once per 30min). At >DISK_CRIT_PCT: same
#   deep reclaim plus a critical alert (max once per 15min).
#   Always writes disk% to DISK_STATUS_FILE for status-page consumption.
#
#   HARD RULE: this script MUST NEVER stop, pause, or otherwise touch the
#   GitHub Actions / Forgejo runner service. Pausing the runner to "protect"
#   disk deadlocks ALL org CI — proven live on nself-sentry 2026-08-15/16
#   (runner paused at the critical threshold, ~15 hours of org-wide CI
#   downtime before the pause was found and reverted). A full disk fails the
#   in-flight job, which retries; a paused runner fails every job, forever,
#   until a human notices. Reclaim harder instead of pausing.
#
# Usage (cron entry):
#   */5 * * * * /opt/nself-ops/bin/disk-guard.sh >>/opt/nself-ops/errors/.disk-guard.log 2>&1
#
# Env vars (all have defaults):
#   REPORT_DIR         — where to write MD alerts        (default: /opt/nself-ops/errors)
#   DISK_WARN_PCT      — warn / proactive-reclaim thresh  (default: 80)
#   DISK_CRIT_PCT      — critical alert threshold         (default: 90)
#   DISK_STATUS_FILE   — JSON status file for status page (default: /opt/nself-ops/status/disk.json)
#   OPS_REPOS_DIR      — build workspace to sweep for      (default: /opt/nself-ops/repos)
#                        regenerable artifacts (node_modules/target/.next/dist)
#
# Dedup:
#   WARN  alert: 30-min TTL lockfile at /tmp/disk-guard-warn.lock
#   CRIT  alert: 15-min TTL lockfile at /tmp/disk-guard-crit.lock

set -euo pipefail

REPORT_DIR="${REPORT_DIR:-/opt/nself-ops/errors}"
DISK_WARN_PCT="${DISK_WARN_PCT:-80}"
DISK_CRIT_PCT="${DISK_CRIT_PCT:-90}"
DISK_STATUS_FILE="${DISK_STATUS_FILE:-/opt/nself-ops/status/disk.json}"
OPS_REPOS_DIR="${OPS_REPOS_DIR:-/opt/nself-ops/repos}"

WARN_DEDUP_TTL=1800   # 30 minutes in seconds
CRIT_DEDUP_TTL=900    # 15 minutes in seconds

mkdir -p "$REPORT_DIR"

ts_iso()  { date -u +%FT%TZ; }
ts_file() { date -u +%Y%m%d-%H%M%S; }
hash6()   { printf "%s" "$1" | md5sum | cut -c1-6; }

# write_status <used_pct>
write_status() {
  local used_pct="$1"
  local dir; dir=$(dirname "$DISK_STATUS_FILE")
  mkdir -p "$dir"
  printf '{"disk_pct": %s, "updated_at": "%s", "threshold_warn": %s, "threshold_crit": %s}\n' \
    "$used_pct" "$(ts_iso)" "$DISK_WARN_PCT" "$DISK_CRIT_PCT" > "$DISK_STATUS_FILE"
}

# emit_warn <used_pct>
emit_warn() {
  local used_pct="$1"
  local lock="/tmp/disk-guard-warn.lock"

  if [ -f "$lock" ]; then
    local age=$(( $(date +%s) - $(date -r "$lock" +%s 2>/dev/null || echo 0) ))
    if [ "$age" -lt "$WARN_DEDUP_TTL" ]; then
      echo "[disk-guard] dedup skip: warn alert already fired ${age}s ago" >&2
      return 0
    fi
  fi

  local df_out; df_out=$(df -h / 2>&1 || true)
  local key="disk-guard:warn:$(ts_file)"
  local h; h=$(hash6 "$key")
  local f="${REPORT_DIR}/$(ts_file)-${h}-disk-guard-warn.md"

  {
    echo "---"
    echo "id: ${key}"
    echo "created_at: $(ts_iso)"
    echo "title: \"Disk usage at ${used_pct}% — pre-emptive alert on sentry box\""
    echo "severity: high"
    echo "source: disk-guard"
    echo "---"
    echo
    echo "# Disk usage at ${used_pct}% — pre-emptive alert on sentry box"
    echo
    echo "Disk usage on \`/\` has reached ${used_pct}% (warn threshold: ${DISK_WARN_PCT}%)."
    echo "disk-guard.sh already ran deep_reclaim automatically (see disk-prune.sh log"
    echo "at /opt/nself-ops/errors/.disk-prune.log). If disk is still climbing:"
    echo
    echo "\`\`\`"
    echo "$df_out"
    echo "\`\`\`"
    echo
    echo "Investigate further:"
    echo
    echo "\`\`\`bash"
    echo "# largest consumers"
    echo "du -sh /var/lib/docker/* 2>/dev/null | sort -rh | head -20"
    echo "docker system df"
    echo ""
    echo "# manual reclaim (never add --volumes — risks deleting DB data)"
    echo "docker system prune -af"
    echo ""
    echo "# runner workspace"
    echo "du -sh /home/runner/_work /opt/actions-runner/_work 2>/dev/null"
    echo "\`\`\`"
  } > "$f"

  touch "$lock"
  echo "[disk-guard] warn alert written: $f" >&2
}

# emit_crit <used_pct>
emit_crit() {
  local used_pct="$1"
  local lock="/tmp/disk-guard-crit.lock"

  if [ -f "$lock" ]; then
    local age=$(( $(date +%s) - $(date -r "$lock" +%s 2>/dev/null || echo 0) ))
    if [ "$age" -lt "$CRIT_DEDUP_TTL" ]; then
      echo "[disk-guard] dedup skip: crit alert already fired ${age}s ago" >&2
      return 0
    fi
  fi

  local df_out; df_out=$(df -h / 2>&1 || true)
  local key="disk-guard:crit:$(ts_file)"
  local h; h=$(hash6 "$key")
  local f="${REPORT_DIR}/$(ts_file)-${h}-disk-guard-crit.md"

  {
    echo "---"
    echo "id: ${key}"
    echo "created_at: $(ts_iso)"
    echo "title: \"CRITICAL: Disk at ${used_pct}% — protective actions taken\""
    echo "severity: critical"
    echo "source: disk-guard"
    echo "---"
    echo
    echo "# CRITICAL: Disk at ${used_pct}% — protective actions taken"
    echo
    echo "Disk usage on \`/\` has reached ${used_pct}% (critical threshold: ${DISK_CRIT_PCT}%)."
    echo "disk-guard.sh ran deep_reclaim (see below). Runners were NOT paused —"
    echo "pausing them deadlocks all org CI; see the header of this script."
    echo
    echo "1. **Docker prune** — ran \`docker system prune -af\` (images, stopped"
    echo "   containers, networks, build cache — never \`--volumes\`, which would"
    echo "   risk deleting DB data volumes)."
    echo "2. **Docker builder prune** — ran \`docker builder prune -af\` to reclaim"
    echo "   buildx/BuildKit cache layers."
    echo "3. **Runner workspace cleanup** — wiped idle \`_work/\` subdirectories and"
    echo "   stale tool-cache/externals under the runner's work dir."
    echo "4. **Build cache sweep** — cleared Go build/module cache, cargo registry"
    echo "   and git-checkout caches, and regenerable artifacts (\`node_modules\`,"
    echo "   \`target\`, \`.next\`, \`dist\`) under \`${OPS_REPOS_DIR}\`."
    echo "5. **journalctl vacuum** — trimmed systemd journal logs to 100M."
    echo
    echo "**Current disk state after reclaim:**"
    echo
    echo "\`\`\`"
    echo "$df_out"
    echo "\`\`\`"
    echo
    echo "If disk is still critical, check for large log files or core dumps:"
    echo
    echo "\`\`\`bash"
    echo "find /var/log /opt/nself-ops/errors -name '*.log' -size +100M 2>/dev/null | sort"
    echo "find /tmp /var/tmp -size +500M 2>/dev/null | sort"
    echo "\`\`\`"
  } > "$f"

  touch "$lock"
  echo "[disk-guard] crit alert written: $f" >&2
}

# deep_reclaim: full reclaim sweep. Runners are NEVER paused — see file header.
# Used at both WARN (proactive) and CRIT (still just reclaim, no pause).
deep_reclaim() {
  echo "[disk-guard] running deep reclaim" >&2

  # 1. Delegate to disk-prune.sh for docker prune + runner _work cleanup +
  #    containerized-runner _work cleanup (force-run regardless of its own
  #    threshold check, since disk-guard already decided reclaim is needed).
  if [ -x /opt/nself-ops/bin/disk-prune.sh ]; then
    DISK_PRUNE_FORCE=1 /opt/nself-ops/bin/disk-prune.sh 2>&1 || true
  elif [ -x "$(dirname "$0")/disk-prune.sh" ]; then
    DISK_PRUNE_FORCE=1 "$(dirname "$0")/disk-prune.sh" 2>&1 || true
  fi

  # 2. Trim the systemd journal — logs are not needed once past 100M. The
  #    remaining reclaim steps (docker builder prune, stale runner tool-cache/
  #    externals, Go/cargo build caches, regenerable artifacts under
  #    OPS_REPOS_DIR) live in disk-prune.sh (step 1 above) so there is one
  #    implementation, not two — see disk-prune.sh for the full list.
  journalctl --vacuum-size=100M 2>&1 || true

  echo "[disk-guard] deep reclaim done" >&2
}

main() {
  local used_pct
  used_pct=$(df / | awk 'NR==2 {gsub(/%/,"",$5); print $5}') || true

  if [ -z "$used_pct" ]; then
    echo "[disk-guard] could not determine disk usage, skipping" >&2
    exit 0
  fi

  echo "[disk-guard] disk usage: ${used_pct}%"

  # Always write status file (status page reads this every run)
  write_status "$used_pct"

  if [ "$used_pct" -ge "$DISK_CRIT_PCT" ]; then
    echo "[disk-guard] CRITICAL: disk at ${used_pct}% (threshold ${DISK_CRIT_PCT}%)" >&2
    deep_reclaim
    emit_crit "$used_pct"
    emit_warn "$used_pct"   # ensure warn fires too (separate dedup key)
  elif [ "$used_pct" -ge "$DISK_WARN_PCT" ]; then
    echo "[disk-guard] WARNING: disk at ${used_pct}% (threshold ${DISK_WARN_PCT}%)" >&2
    # Reclaim proactively at WARN, not just CRIT — closes the gap where CI
    # backlog fills the disk between the (formerly 20-min, now 10-min)
    # disk-prune.sh cron runs. See file header.
    deep_reclaim
    emit_warn "$used_pct"
  else
    echo "[disk-guard] disk OK (${used_pct}% < ${DISK_WARN_PCT}%)"
  fi

  echo "[disk-guard] done at $(ts_iso)"
}

main
