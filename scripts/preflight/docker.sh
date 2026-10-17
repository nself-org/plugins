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

# pf_cli_dockerfile ROOT MANIFEST OUT: write OUT, a Dockerfile that builds a cli
# plugin's command binaries from the extracted tree alone with the release recipe
# (scripts/build-tarballs.sh build_platform_tarballs: CGO_ENABLED=0 go build of
# each target from cmd/<command>/, else cmd/). The targets are the release's own:
# scripts/release/cli-targets.sh over the manifest's directory, so a v1 or a v2
# manifest resolves exactly as the installer and the release resolve it.
# Used when a cli plugin ships no Dockerfile: its release is per-platform binary
# tarballs, so the source has nothing else to build.
# Returns 1 and prints the reason when the tarball cannot express that build.
pf_cli_dockerfile() {
  local root="$1" manifest="$2" out="$3" gover targets err tab bin cmd pkg
  if [ ! -f "$root/go.mod" ]; then
    echo "cli plugin tarball has no Dockerfile and no go.mod: nothing to build from the tarball alone"
    return 1
  fi
  gover=$(awk '$1 == "go" {print $2; exit}' "$root/go.mod")
  if ! printf '%s' "$gover" | grep -Eq '^[0-9]+\.[0-9]+(\.[0-9]+)?$'; then
    echo "cli plugin go.mod has no usable go directive (${gover:-none})"
    return 1
  fi
  err="$out.err"
  if ! targets=$(bash "$(dirname "${BASH_SOURCE[0]}")/../release/cli-targets.sh" "$(dirname "$manifest")" 2>"$err"); then
    echo "cli plugin manifest refused by cli-targets.sh: $(head -n 1 "$err")"
    rm -f "$err"
    return 1
  fi
  rm -f "$err"
  [ -n "$targets" ] || { echo "cli plugin declares no command binary to build"; return 1; }
  # Every line is "nself-<command><TAB><command>" with a plain command name: a split,
  # numeric-led, empty or path-like name is rejected, as the release gate rejects it.
  tab=$(printf '\t')
  while IFS="$tab" read -r bin cmd; do
    if ! printf '%s' "$cmd" | grep -Eq '^[a-z][a-z0-9-]*$' || [ "$bin" != "nself-$cmd" ]; then
      echo "cli command target [$bin] [$cmd] is not a plain nself-<command> name"
      return 1
    fi
  done <<<"$targets"
  {
    printf 'FROM golang:%s-alpine\nWORKDIR /src\nCOPY . .\nENV CGO_ENABLED=0\n' "$gover"
    [ ! -d "$root/vendor" ] || printf 'ENV GOFLAGS=-mod=vendor\n'
    while IFS="$tab" read -r bin cmd; do
      pkg="./cmd/"
      [ ! -d "$root/cmd/$cmd" ] || pkg="./cmd/$cmd/"
      printf 'RUN go build -trimpath -ldflags="-s -w" -o /out/%s %s\n' "$bin" "$pkg"
    done <<<"$targets"
  } >"$out"
}

# pf_build ROOT SHA SLUG [DOCKERFILE]: docker build from the extracted tree
# alone (with DOCKERFILE in place of ROOT/Dockerfile when given). Sets
# PF_B_RESULT PF_B_CLASS PF_B_REASON. Identical bytes (same sha256) are built
# once; the second source reuses the measured outcome. With PF_BUILD_CACHE
# (--build-cache DIR) outcomes persist so an interrupted run resumes.
# A daemon outage is infrastructure, never a plugin build failure: the build is
# retried after the daemon returns, else the run aborts (exit 2).
pf_build() {
  local root="$1" sha="$2" slug="$3" df="${4:-}" cache rc=0 tag log err cause blk tries=0 short
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
    [ -z "$df" ] || cmd+=(-f "$df")
    ${PF_TIMEOUT_BIN:+$PF_TIMEOUT_BIN ${PF_BUILD_TIMEOUT:-1800}} \
      "${cmd[@]}" --label "nself.preflight=$PF_RUN_ID" --progress=plain -t "$tag" "$root" >"$log" 2>&1 || rc=$?
    [ "$rc" != 0 ] || break
    grep -Eq 'Docker Desktop is unable to start|failed to connect to the docker API|Cannot connect to the Docker daemon|error during connect|code = Unavailable|no builder .* found|container .* is not running' "$log" || break
    tries=$((tries + 1))
    [ "$tries" -lt 3 ] || pf_die "docker daemon keeps dropping during $slug; aborting (not a plugin failure)"
    pf_log "docker daemon dropped during $slug (try $tries); retrying"
  done
  if [ "$rc" = 0 ]; then
    PF_B_RESULT=pass PF_B_CLASS=- PF_B_REASON="docker build from the extracted tree succeeded${df:+ (cli release recipe: go build of the command binaries)}"
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
