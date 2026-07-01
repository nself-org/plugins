#!/usr/bin/env bash
# disk-prune.sh — sentry-box disk reclamation + Docker/runner cleanup
#
# Purpose:
#   1. Checks disk usage on /. Emits a deduped MD alert (max once per 6h) if
#      usage exceeds DISK_WARN_PCT.
#   2. Runs docker system prune to reclaim space from stopped containers,
#      dangling images, unused networks, and unused volumes.
#   3. Cleans GitHub Actions runner _work directories that are not currently
#      in use (safe between jobs).
#
# Usage (cron entry):
#   0 * * * * /opt/nself-ops/bin/disk-prune.sh >>/opt/nself-ops/errors/.disk-prune.log 2>&1
#
# Env vars (all have defaults):
#   REPORT_DIR       — where to write MD alerts      (default: /opt/nself-ops/errors)
#   DISK_WARN_PCT    — alert threshold (percent)      (default: 80)
#   RUNNER_WORK_DIR  — GitHub runner _work path       (auto-detect; or set explicitly)

set -euo pipefail

REPORT_DIR="${REPORT_DIR:-/opt/nself-ops/errors}"
DISK_WARN_PCT="${DISK_WARN_PCT:-80}"
DEDUP_TTL=21600  # 6 hours in seconds

mkdir -p "$REPORT_DIR"

ts_iso()  { date -u +%FT%TZ; }
ts_file() { date -u +%Y%m%d-%H%M%S; }
hash6()   { printf "%s" "$1" | md5sum | cut -c1-6; }

# Auto-detect runner work dir if not set
if [ -z "${RUNNER_WORK_DIR:-}" ]; then
  if [ -d "/home/runner/_work" ]; then
    RUNNER_WORK_DIR="/home/runner/_work"
  elif [ -d "/opt/actions-runner/_work" ]; then
    RUNNER_WORK_DIR="/opt/actions-runner/_work"
  else
    RUNNER_WORK_DIR=""
  fi
fi

emit_disk_alert() {
  local used_pct="$1"
  local lock="/tmp/disk-prune-disk-full.lock"

  # dedup: one alert per DEDUP_TTL
  if [ -f "$lock" ]; then
    local age=$(( $(date +%s) - $(date -r "$lock" +%s 2>/dev/null || echo 0) ))
    if [ "$age" -lt "$DEDUP_TTL" ]; then
      echo "[disk-prune] dedup skip: disk-full alert already fired ${age}s ago" >&2
      return 0
    fi
  fi

  local df_out; df_out=$(df -h / 2>&1 || true)
  local key="disk-prune:disk-full:$(ts_file)"
  local h; h=$(hash6 "$key")
  local f="${REPORT_DIR}/$(ts_file)-${h}-disk-prune-disk-full.md"

  {
    echo "---"
    echo "id: ${key}"
    echo "created_at: $(ts_iso)"
    echo "title: \"Disk usage at ${used_pct}% on sentry box\""
    echo "severity: high"
    echo "source: disk-prune"
    echo "---"
    echo
    echo "# Disk usage at ${used_pct}% on sentry box"
    echo
    echo "Disk usage on \`/\` has exceeded the ${DISK_WARN_PCT}% threshold."
    echo
    echo "\`\`\`"
    echo "$df_out"
    echo "\`\`\`"
    echo
    echo "docker system prune was run automatically. If disk is still critical:"
    echo
    echo "\`\`\`bash"
    echo "du -sh /var/lib/docker/* 2>/dev/null | sort -rh | head -20"
    echo "docker system df"
    echo "\`\`\`"
  } > "$f"

  touch "$lock"
  echo "[disk-prune] disk alert written: $f" >&2
}

check_disk() {
  local used_pct
  # df output: "Use%" column; strip the % sign
  used_pct=$(df / | awk 'NR==2 {gsub(/%/,"",$5); print $5}') || true
  if [ -z "$used_pct" ]; then
    echo "[disk-prune] could not determine disk usage, skipping check" >&2
    return 0
  fi
  echo "[disk-prune] disk usage: ${used_pct}%"
  if [ "$used_pct" -ge "$DISK_WARN_PCT" ]; then
    echo "[disk-prune] WARNING: disk at ${used_pct}% (threshold ${DISK_WARN_PCT}%)" >&2
    emit_disk_alert "$used_pct"
  fi
}

prune_docker() {
  echo "[disk-prune] running docker system prune..."
  local result
  # NOTE: deliberately NO --volumes — §12 bans volume-destructive pruning
  # (a stopped runner's config volume or a data volume must never be reaped
  # by a cleanup cron). Images, stopped containers, networks, build cache only.
  result=$(docker system prune -af 2>&1) || true
  echo "[disk-prune] docker prune result:"
  echo "$result"
}

# prune_containerized_runners cleans _work INSIDE runner containers (named
# volumes like gh-runner-*-data that host-path cleanup can't reach — the
# cam-sentry Rust-build disk-fill failure mode). A runner with a live job
# (Runner.Worker process) is skipped.
prune_containerized_runners() {
  local runners
  runners=$(docker ps --format '{{.Names}}\t{{.Image}}' 2>/dev/null | \
    awk -F'\t' 'tolower($0) ~ /actions-runner|github-runner|gh-runner|myoung34/ {print $1}') || true
  [ -z "$runners" ] && { echo "[disk-prune] no containerized runners found, skipping"; return 0; }

  for c in $runners; do
    if docker exec "$c" sh -c 'ps -ef 2>/dev/null || ps aux' 2>/dev/null | grep -q 'Runner.Worker'; then
      echo "[disk-prune] skipping $c (job running)"
      continue
    fi
    echo "[disk-prune] cleaning _work inside $c"
    docker exec "$c" sh -c \
      'for d in /actions-runner/_work /runner/_work /home/runner/_work; do
         [ -d "$d" ] && rm -rf "$d"/* 2>/dev/null
       done; true' 2>/dev/null || echo "[disk-prune] warn: cleanup failed inside $c" >&2
  done
}

prune_runner_work() {
  if [ -z "$RUNNER_WORK_DIR" ] || [ ! -d "$RUNNER_WORK_DIR" ]; then
    echo "[disk-prune] no runner work dir found, skipping"
    return 0
  fi

  echo "[disk-prune] scanning runner work dir: $RUNNER_WORK_DIR"

  # Iterate top-level dirs (repo workspace dirs) and clean if not in use
  while IFS= read -r -d '' dir; do
    [ -d "$dir" ] || continue

    # Check if any process has open files in this dir (safe to clean if none)
    local in_use=0
    if command -v lsof >/dev/null 2>&1; then
      lsof_out=$(lsof +D "$dir" 2>/dev/null | grep -v "^COMMAND" || true)
      if [ -n "$lsof_out" ]; then
        in_use=1
      fi
    fi

    if [ "$in_use" -eq 1 ]; then
      echo "[disk-prune] skipping (in use): $dir" >&2
    else
      echo "[disk-prune] cleaning: $dir"
      rm -rf "${dir:?}"/* 2>/dev/null || true
    fi
  done < <(find "$RUNNER_WORK_DIR" -mindepth 1 -maxdepth 1 -type d -print0 2>/dev/null)
}

check_disk
prune_docker
prune_runner_work
prune_containerized_runners
echo "[disk-prune] done at $(ts_iso)"
