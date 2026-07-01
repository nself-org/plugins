#!/usr/bin/env bash
# disk-guard.sh — pre-emptive disk alert + protective action on sentry box
#
# Purpose:
#   Runs every 5 minutes (cron IS the loop, this is single-shot). Checks disk
#   usage on /. At >DISK_WARN_PCT: emits a deduped MD alert (max once per 30min).
#   At >DISK_CRIT_PCT: runs aggressive prune, pauses the GitHub Actions runner
#   service, and emits a critical alert (max once per 15min).
#   Always writes disk% to DISK_STATUS_FILE for status-page consumption.
#
# Usage (cron entry):
#   */5 * * * * /opt/nself-ops/bin/disk-guard.sh >>/opt/nself-ops/errors/.disk-guard.log 2>&1
#
# Env vars (all have defaults):
#   REPORT_DIR         — where to write MD alerts        (default: /opt/nself-ops/errors)
#   DISK_WARN_PCT      — warn threshold (percent)         (default: 80)
#   DISK_CRIT_PCT      — critical/protect threshold       (default: 90)
#   DISK_STATUS_FILE   — JSON status file for status page (default: /opt/nself-ops/status/disk.json)
#   RUNNER_SERVICE     — glob for systemctl unit name     (default: actions.runner.*.service)
#
# Dedup:
#   WARN  alert: 30-min TTL lockfile at /tmp/disk-guard-warn.lock
#   CRIT  alert: 15-min TTL lockfile at /tmp/disk-guard-crit.lock

set -euo pipefail

REPORT_DIR="${REPORT_DIR:-/opt/nself-ops/errors}"
DISK_WARN_PCT="${DISK_WARN_PCT:-80}"
DISK_CRIT_PCT="${DISK_CRIT_PCT:-90}"
DISK_STATUS_FILE="${DISK_STATUS_FILE:-/opt/nself-ops/status/disk.json}"
RUNNER_SERVICE="${RUNNER_SERVICE:-actions.runner.*.service}"

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
    echo "disk-guard.sh fires every 5 minutes; disk-prune.sh (hourly) may not reclaim fast enough."
    echo
    echo "\`\`\`"
    echo "$df_out"
    echo "\`\`\`"
    echo
    echo "Investigate and reclaim space:"
    echo
    echo "\`\`\`bash"
    echo "# largest consumers"
    echo "du -sh /var/lib/docker/* 2>/dev/null | sort -rh | head -20"
    echo "docker system df"
    echo ""
    echo "# manual reclaim"
    echo "docker system prune -af --volumes"
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
    echo "disk-guard.sh automatically took the following protective actions:"
    echo
    echo "1. **Docker aggressive prune** — ran \`docker system prune -af --volumes\` to"
    echo "   remove all stopped containers, unused images, networks, and volumes."
    echo "2. **Runner workspace cleanup** — wiped idle \`_work/\` subdirectories under"
    echo "   \`/home/runner/_work\` or \`/opt/actions-runner/_work\`."
    echo "3. **Runner service pause** — issued \`systemctl stop\` for any"
    echo "   \`${RUNNER_SERVICE}\` units found, to prevent new CI jobs from filling"
    echo "   the disk further. Restart with:"
    echo
    echo "\`\`\`bash"
    echo "systemctl start '${RUNNER_SERVICE}'"
    echo "\`\`\`"
    echo
    echo "**Current disk state after protective actions:**"
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

# protect: aggressive prune + runner workspace clean + pause runner service
protect() {
  echo "[disk-guard] CRITICAL threshold reached — running protective actions" >&2

  # 1. Aggressive Docker prune
  echo "[disk-guard] docker system prune -af --volumes..."
  docker system prune -af --volumes 2>&1 || true

  # 2. Clean runner _work (same auto-detect as disk-prune.sh)
  local runner_work=""
  if [ -d "/home/runner/_work" ]; then
    runner_work="/home/runner/_work"
  elif [ -d "/opt/actions-runner/_work" ]; then
    runner_work="/opt/actions-runner/_work"
  fi

  if [ -n "$runner_work" ] && [ -d "$runner_work" ]; then
    echo "[disk-guard] cleaning runner work dir: $runner_work"
    find "$runner_work" -mindepth 1 -maxdepth 1 -type d -print0 2>/dev/null \
      | while IFS= read -r -d '' dir; do
          rm -rf "${dir:?}"/* 2>/dev/null || true
          echo "[disk-guard] cleaned: $dir"
        done
  else
    echo "[disk-guard] no runner work dir found, skipping"
  fi

  # 3. Pause runner service(s)
  local units
  units=$(systemctl list-units --plain --no-legend "${RUNNER_SERVICE}" 2>/dev/null \
          | awk '{print $1}') || true
  if [ -n "$units" ]; then
    for unit in $units; do
      systemctl stop "$unit" 2>/dev/null && echo "[disk-guard] paused runner: $unit" || true
    done
  else
    echo "[disk-guard] no runner service units found matching: ${RUNNER_SERVICE}"
  fi
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
    protect
    emit_crit "$used_pct"
    emit_warn "$used_pct"   # ensure warn fires too (separate dedup key)
  elif [ "$used_pct" -ge "$DISK_WARN_PCT" ]; then
    echo "[disk-guard] WARNING: disk at ${used_pct}% (threshold ${DISK_WARN_PCT}%)" >&2
    emit_warn "$used_pct"
  else
    echo "[disk-guard] disk OK (${used_pct}% < ${DISK_WARN_PCT}%)"
  fi

  echo "[disk-guard] done at $(ts_iso)"
}

main
