#!/usr/bin/env bash
# docker.sh - docker side of the preflight (P7-PLUG-02). Sourced by run.sh.
#
# Everything created here is scoped to the run (PF_RUN_ID): a dedicated buildx
# builder named nself-preflight-<run-id> (its build cache is removed with it)
# and images labelled nself.preflight=<run-id>. No global prune of any kind
# runs: other projects' images, containers and build cache are never touched.

# pf_docker_up: wait (default 600 s) for the docker daemon; false when it stays down.
pf_docker_up() {
  local waited=0
  while ! docker info >/dev/null 2>&1; do
    [ "$waited" -lt "${PF_DOCKER_WAIT:-600}" ] || return 1
    [ "$waited" != 0 ] || pf_log "docker daemon unreachable; waiting up to ${PF_DOCKER_WAIT:-600}s"
    sleep 10
    waited=$((waited + 10))
  done
}

# pf_builder_ensure: create (or recreate after a daemon restart) the run's own
# builder. PF_BUILDER stays empty when buildx is unavailable: builds then use the
# default builder and the build cache is simply never pruned.
pf_builder_ensure() {
  [ "${PF_NO_BUILD:-0}" != 1 ] || return 0
  docker buildx version >/dev/null 2>&1 || { PF_BUILDER=""; return 0; }
  PF_BUILDER="nself-preflight-$PF_RUN_ID"
  docker buildx inspect "$PF_BUILDER" >/dev/null 2>&1 && return 0
  # Remember whether the buildkit image pre-existed: if buildx pulls it for us, cleanup removes it.
  [ -n "${PF_BK_KNOWN:-}" ] || { PF_BK_KNOWN=1; PF_BK_BEFORE=$(docker image ls -q moby/buildkit 2>/dev/null | sort -u | tr '\n' ' '); }
  docker buildx create --name "$PF_BUILDER" --driver docker-container >/dev/null 2>&1 \
    && docker buildx inspect --bootstrap --builder "$PF_BUILDER" >/dev/null 2>&1 \
    || { pf_log "buildx builder unavailable; using the default builder"; PF_BUILDER=""; }
}

# pf_docker_cleanup: remove only what this run created. Safe to call twice.
pf_docker_cleanup() {
  local ids
  command -v docker >/dev/null 2>&1 || return 0
  [ -n "${PF_RUN_ID:-}" ] || return 0
  ids=$(docker image ls -q --filter "label=nself.preflight=$PF_RUN_ID" 2>/dev/null | sort -u | tr '\n' ' ')
  [ -z "$ids" ] || docker rmi -f $ids >/dev/null 2>&1 || true
  [ -z "${PF_BUILDER:-}" ] || docker buildx rm "$PF_BUILDER" >/dev/null 2>&1 || true
  if [ -n "${PF_BK_KNOWN:-}" ]; then
    for ids in $(docker image ls -q moby/buildkit 2>/dev/null | sort -u); do
      case " ${PF_BK_BEFORE:-} " in *" $ids "*) ;; *) docker rmi "$ids" >/dev/null 2>&1 || true ;; esac
    done
  fi
}

# pf_build ROOT SHA SLUG: docker build from the extracted tree alone. Sets
# PF_B_RESULT PF_B_CLASS PF_B_REASON. Identical bytes (same sha256) are built
# once; the second source reuses the measured outcome. With PF_BUILD_CACHE
# (--build-cache DIR) outcomes persist so an interrupted run resumes.
# A daemon outage is infrastructure, never a plugin build failure: the build is
# retried after the daemon returns, else the run aborts (exit 2).
pf_build() {
  local root="$1" sha="$2" slug="$3" cache rc=0 tag log err cause blk tries=0 short
  local -a cmd
  short=$(printf '%s' "$sha" | cut -c1-12)
  cache="$PF_WORK/build/$sha.res"
  [ -f "$cache" ] || { [ -z "${PF_BUILD_CACHE:-}" ] || cache="$PF_BUILD_CACHE/$sha.res"; }
  if [ -f "$cache" ]; then
    IFS=$'\t' read -r PF_B_RESULT PF_B_CLASS PF_B_REASON <"$cache"
    return 0
  fi
  cache="$PF_WORK/build/$sha.res"
  if [ "${PF_NO_BUILD:-0}" = 1 ]; then
    PF_B_RESULT=skip PF_B_CLASS=build PF_B_REASON="docker build skipped (--no-build); a skip is not a pass"
    return 0
  fi
  tag="nself-preflight/${slug}:$short"
  log="$PF_WORK/logs/${slug}-$short.log"
  while :; do
    rc=0
    pf_docker_up || pf_die "docker daemon down; aborting (not a plugin failure)"
    pf_builder_ensure
    if [ -n "${PF_BUILDER:-}" ]; then cmd=(docker buildx build --builder "$PF_BUILDER" --load); else cmd=(docker build); fi
    ${PF_TIMEOUT_BIN:+$PF_TIMEOUT_BIN ${PF_BUILD_TIMEOUT:-1800}} \
      "${cmd[@]}" --label "nself.preflight=$PF_RUN_ID" --progress=plain -t "$tag" "$root" >"$log" 2>&1 || rc=$?
    [ "$rc" != 0 ] || break
    grep -Eq 'Docker Desktop is unable to start|failed to connect to the docker API|Cannot connect to the Docker daemon|error during connect|code = Unavailable|no builder .* found|container .* is not running' "$log" || break
    tries=$((tries + 1))
    [ "$tries" -lt 3 ] || pf_die "docker daemon keeps dropping during $slug; aborting (not a plugin failure)"
    pf_log "docker daemon dropped during $slug (try $tries); retrying"
  done
  if [ "$rc" = 0 ]; then
    PF_B_RESULT=pass PF_B_CLASS=- PF_B_REASON="docker build from the extracted tree succeeded"
    docker rmi -f "$tag" >/dev/null 2>&1 || true
  else
    if [ "$rc" = 124 ]; then
      PF_B_REASON="docker build timed out after ${PF_BUILD_TIMEOUT:-1800}s"
    else
      # Fixed text only: strip buildkit ref ids and step timestamps so two runs match.
      err=$( (grep '^ERROR' "$log" || true) | tail -n 1 | sed -E 's/ of ref [a-z0-9:]+//; s/^ERROR: failed to build: failed to solve: //' | tr -s ' ' | cut -c1-150)
      blk=$(awk '/^ > \[/ {b = 1; next} /^------$/ {b = 0} b' "$log" | sed -E 's/^[0-9]+\.[0-9]+ +//')
      cause=$( (printf '%s\n' "$blk" | grep -iE 'is required|requires go|not found|cannot|unable' || true) | head -n 1 | tr -s ' ' | cut -c1-160)
      [ -n "$cause" ] || cause=$( (printf '%s\n' "$blk" | grep -iE 'error|failed' || true) | head -n 1 | sed -E 's/ at `[^`]*`/ at <path>/' | tr -s ' ' | cut -c1-160)
      PF_B_REASON="docker build exit $rc${err:+: $err}${cause:+ | cause: $cause}"
    fi
    PF_B_RESULT=fail PF_B_CLASS=build
    [ -z "${PF_LOG_DIR:-}" ] || { mkdir -p "$PF_LOG_DIR" && cp "$log" "$PF_LOG_DIR/"; }
  fi
  printf '%s\t%s\t%s\n' "$PF_B_RESULT" "$PF_B_CLASS" "$PF_B_REASON" >"$cache"
  [ -z "${PF_BUILD_CACHE:-}" ] || { mkdir -p "$PF_BUILD_CACHE" && cp "$cache" "$PF_BUILD_CACHE/$sha.res"; }
  PF_BUILD_COUNT=$((${PF_BUILD_COUNT:-0} + 1))
  # Scoped housekeeping only: this run's own dangling images and its own builder's cache.
  docker image prune -f --filter "label=nself.preflight=$PF_RUN_ID" >/dev/null 2>&1 || true
  if [ -n "${PF_BUILDER:-}" ] && [ $((PF_BUILD_COUNT % ${PF_PRUNE_EVERY:-10})) = 0 ]; then
    docker buildx prune --builder "$PF_BUILDER" -f --max-used-space 4GB >/dev/null 2>&1 \
      || docker buildx prune --builder "$PF_BUILDER" -f --keep-storage 4GB >/dev/null 2>&1 || true
  fi
}
