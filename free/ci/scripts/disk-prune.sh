#!/usr/bin/env bash
# disk-prune.sh — sentry-box deep reclaim: Docker/runner/build-cache cleanup
#
# Purpose:
#   1. Checks disk usage on /. Emits a deduped MD alert (max once per 6h) if
#      usage exceeds DISK_WARN_PCT.
#   2. Runs docker system prune (images, containers, networks — never
#      --volumes, which risks deleting DB data volumes; see nsentry-server-
#      standard.md §12) and docker builder prune (BuildKit cache).
#   3. Cleans GitHub Actions runner _work directories that are not currently
#      in use (safe between jobs), plus stale tool-cache/externals dirs.
#   4. Sweeps regenerable build caches (Go build/module cache, cargo registry
#      + git-checkout caches) and regenerable artifacts (node_modules/target/
#      .next/dist) under OPS_REPOS_DIR, the ops box's build workspace.
#
#   All actions here are non-destructive to persistent data: everything
#   removed is either a cache or an artifact that rebuilds/reinstalls from
#   source on the next job. This script NEVER touches the runner service
#   itself — reclaim is what protects disk, not pausing CI. See disk-guard.sh
#   header for the incident this rule prevents recurring (2026-08-15/16).
#
# Usage (cron entry — every 10 min; was hourly, tightened after the 2026-08-15
# incident showed CI backlog can fill disk faster than an hourly cadence
# reclaims it):
#   */10 * * * * /opt/nself-ops/bin/disk-prune.sh >>/opt/nself-ops/errors/.disk-prune.log 2>&1
#
# disk-guard.sh (runs every 5 min) also calls this script directly with
# DISK_PRUNE_FORCE=1 whenever disk crosses DISK_WARN_PCT, so reclaim happens
# proactively between cron ticks too — DISK_PRUNE_FORCE is accepted for that
# call site; this script's prune/sweep steps always run on every invocation
# regardless (only the alert-email dedup is threshold-gated).
#
# Env vars (all have defaults):
#   REPORT_DIR       — where to write MD alerts      (default: /opt/nself-ops/errors)
#   DISK_WARN_PCT    — alert threshold (percent)      (default: 80)
#   RUNNER_WORK_DIR  — GitHub runner _work path       (auto-detect; or set explicitly)
#   OPS_REPOS_DIR    — build workspace to sweep for   (default: /opt/nself-ops/repos)
#                      regenerable artifacts (node_modules/target/.next/dist)
#   DISK_PRUNE_FORCE — set by disk-guard.sh callers; accepted for forward
#                      compatibility (default: 0, no effect on current logic)

set -euo pipefail

REPORT_DIR="${REPORT_DIR:-/opt/nself-ops/errors}"
DISK_WARN_PCT="${DISK_WARN_PCT:-80}"
OPS_REPOS_DIR="${OPS_REPOS_DIR:-/opt/nself-ops/repos}"
DISK_PRUNE_FORCE="${DISK_PRUNE_FORCE:-0}"
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

  echo "[disk-prune] running docker builder prune..."
  docker builder prune -af 2>&1 || true
}

# prune_runner_tool_cache cleans stale GitHub Actions runner tool-cache and
# externals dirs (node/python/etc runtime bundles the runner downloads per
# job). Both auto-redownload on next job start, so wiping them is safe
# between jobs — same "not currently in use" contract as prune_runner_work.
prune_runner_tool_cache() {
  local d
  for d in /home/runner/_work/_tool /opt/actions-runner/_work/_tool \
           /home/runner/externals /opt/actions-runner/externals; do
    if [ -d "$d" ]; then
      echo "[disk-prune] cleaning tool-cache/externals: $d"
      rm -rf "${d:?}"/* 2>/dev/null || true
    fi
  done
}

# prune_build_caches clears Go build/module cache and cargo registry/git
# caches. Both are pure caches — the toolchain repopulates them from the
# module proxy / crates.io on the next build, so this never loses source.
prune_build_caches() {
  local gc
  for gc in /root/.cache/go-build /home/*/.cache/go-build; do
    if [ -d "$gc" ]; then
      echo "[disk-prune] cleaning Go build cache: $gc"
      rm -rf "${gc:?}"/* 2>/dev/null || true
    fi
  done
  if command -v go >/dev/null 2>&1; then
    echo "[disk-prune] go clean -cache -modcache"
    go clean -cache -modcache 2>/dev/null || true
  fi

  # Per-runner GOPATHs. Each self-hosted runner sets its own GOPATH inside its
  # runner directory, so neither the /home/*/.cache/go-build glob above nor
  # `go clean -modcache` (which only touches the invoking user's GOPATH) ever
  # reclaims them. On nself-sentry that was 4 runners x ~4.8G = ~19G of the
  # 150G disk, never freed, and the single largest contributor to the box
  # sitting at 86-88% while the alerts kept firing.
  #
  # Guarded on live workers for the same reason prune_runner_work is: deleting
  # a module cache under a running Go job fails it mid-build. Skipping here
  # only defers the reclaim to the next idle run.
  # Per-RUNNER granularity, deliberately NOT a single global "any worker live"
  # guard. On a continuously busy box the global form never fires even once:
  # observed on nself-sentry at 94% disk with all 4 runners busy, where the
  # sweep skipped every pass while ~19G of GOPATH sat unreclaimed. Skipping
  # only the runners that are actually executing a job still frees the idle
  # ones, which is what keeps the box off the disk-full alert.
  #
  # A Runner.Worker's argv contains its own runner directory
  # (/home/<user>/actions-runner-N/bin.<ver>/Runner.Worker ...), so busy
  # runners can be identified precisely rather than inferred.
  local busy_dirs
  busy_dirs=$(ps -eo args 2>/dev/null | grep -oE '/home/[^/]+/actions-runner[^/]*/' | sort -u)

  local rd
  for rd in /home/*/actions-runner*/; do
    [ -d "$rd" ] || continue
    case "$rd" in */actions-runner*/) ;; *) continue ;; esac
    if [ -n "$busy_dirs" ] && printf '%s\n' "$busy_dirs" | grep -qxF "$rd"; then
      echo "[disk-prune] skip (job running): ${rd}go"
      continue
    fi
    local sub
    for sub in "${rd}go" "${rd}.cache/go-build"; do
      if [ -d "$sub" ]; then
        echo "[disk-prune] cleaning per-runner Go cache: $sub"
        rm -rf "${sub:?}"/* 2>/dev/null || true
      fi
    done
  done

  local cc
  for cc in /root/.cargo /home/*/.cargo; do
    if [ -d "$cc/registry/cache" ]; then
      echo "[disk-prune] cleaning cargo registry cache: $cc/registry/cache"
      rm -rf "${cc:?}/registry/cache"/* 2>/dev/null || true
    fi
    if [ -d "$cc/git/checkouts" ]; then
      echo "[disk-prune] cleaning cargo git checkouts: $cc/git/checkouts"
      rm -rf "${cc:?}/git/checkouts"/* 2>/dev/null || true
    fi
  done
}

# prune_ops_repos_artifacts sweeps regenerable build output (node_modules,
# target, .next, dist) under the ops box's build workspace — the box builds
# nSentry plugin images and CLI release binaries here, and this is what
# piled up in the 2026-08-15/16 incident (Go build cache + these artifacts).
prune_ops_repos_artifacts() {
  if [ -z "$OPS_REPOS_DIR" ] || [ ! -d "$OPS_REPOS_DIR" ]; then
    echo "[disk-prune] OPS_REPOS_DIR not found ($OPS_REPOS_DIR), skipping"
    return 0
  fi
  echo "[disk-prune] sweeping regenerable artifacts under $OPS_REPOS_DIR"
  find "$OPS_REPOS_DIR" -type d \( -name node_modules -o -name target -o -name .next -o -name dist \) \
    -prune -exec rm -rf {} + 2>/dev/null || true
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

  # If this runner has a live job, do not touch its work dir at all. Per-dir lsof
  # is not sufficient: a job sitting between steps holds no open files, so its
  # caches look idle and get deleted underneath it.
  if pgrep -f "Runner.Worker" >/dev/null 2>&1; then
    echo "[disk-prune] live Runner.Worker present - skipping $RUNNER_WORK_DIR entirely"
    return 0
  fi

  # Iterate top-level dirs (repo workspace dirs) and clean if not in use
  while IFS= read -r -d '' dir; do
    [ -d "$dir" ] || continue

    # NEVER prune runner-internal state. These are not repo checkouts:
    #   _actions  - resolved action code (actions/checkout@vN). Deleting it mid-run
    #               causes "Can't find 'action.yml' ... under _work/_actions/...".
    #   _temp     - includes _runner_file_commands (GITHUB_PATH/GITHUB_ENV writes);
    #               deleting it causes "Missing file at path: .../add_path_<uuid>".
    #   _tool     - hosted tool cache (Go/Node installs) the job resolves once.
    #   _PipelineMapping / _diag - runner bookkeeping.
    # Observed 2026-08-16 on nself-sentry: pruning these broke in-flight jobs
    # across the org (plugins-pro govulncheck failed with both errors above), and
    # was a likely cause of other intermittent CI failures the same day.
    case "$(basename "$dir")" in
      _actions|_temp|_tool|_PipelineMapping|_diag)
        echo "[disk-prune] protected (runner internal): $dir"
        continue
        ;;
    esac

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
prune_runner_tool_cache
prune_build_caches
prune_ops_repos_artifacts
echo "[disk-prune] done at $(ts_iso)"
