#!/usr/bin/env bash
# Purpose: prove the forgejo plugin upgrade (P7-TRUST-21, Epic TRUST D22) on a real Docker daemon. It brings up the
#   previous fragment (git ref FORGEJO_SMOKE_BASE_REF) in a scratch nself project, swaps in this checkout's fragment,
#   runs `nself build --remove-orphans`, then checks the stack: the in-stack runner is gone, no container of the
#   project mounts the Docker socket, forgejo-init exited 0 and sign-up is closed by default.
# Inputs (env, all optional):
#   FORGEJO_SMOKE_BASE_REF  git ref holding the previous plugin files (default origin/main). A ref that already lacks the
#                           runner skips the upgrade leg and checks the fresh-install result only.
#   FORGEJO_SMOKE_WORK      work directory (default: a new mktemp dir under TMPDIR, removed on exit)
#   FORGEJO_SMOKE_WAIT      seconds to wait for forgejo healthy / forgejo-init done (default 180)
# Outputs: exit 0 when every assertion held; 1 on a failed assertion (compose state is printed first); 2 when a tool
#   is missing; 3 on a host that is not Linux (Constitution section 10.3: the compose smoke runs on Linux).
# Constraints: no secret on any argv (the admin password is generated per run and written to the scratch .env); HOME is a
#   fixture dir and DOCKER_CONFIG keeps the real docker context; everything this script starts is removed on exit.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
plugin="$(cd "$here/.." && pwd)"
repo="$(cd "$plugin/../.." && pwd)"
base_ref="${FORGEJO_SMOKE_BASE_REF:-origin/main}"
wait_s="${FORGEJO_SMOKE_WAIT:-180}"
proj_name="fgsmoke"
sock="docker.""sock"

[ "$(uname -s)" = Linux ] || { echo "upgrade-smoke: Linux only (Constitution section 10.3); on another host use the forgejo-smoke.yml pull_request run" >&2; exit 3; }
for t in docker git nself; do command -v "$t" >/dev/null || { echo "upgrade-smoke: $t is required" >&2; exit 2; }; done
if docker compose version >/dev/null 2>&1; then compose=(docker compose); elif command -v docker-compose >/dev/null; then compose=(docker-compose); else echo "upgrade-smoke: docker compose is required" >&2; exit 2; fi

made_work=0
if [ -z "${FORGEJO_SMOKE_WORK:-}" ]; then
  FORGEJO_SMOKE_WORK="$(mktemp -d "${TMPDIR:-/tmp}/forgejo-smoke.XXXXXX")"
  made_work=1
fi
work="$FORGEJO_SMOKE_WORK"
project="$work/project"
rm -rf "${project:?}" "${work:?}/home" "${work:?}/plugins"
mkdir -p "$project" "$work/home" "$work/plugins/forgejo"
export DOCKER_CONFIG="${DOCKER_CONFIG:-$HOME/.docker}"
export HOME="$work/home"
export NSELF_PLUGIN_DIR="$work/plugins"

step() { echo; echo "==> $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# dc runs docker compose over the files and env files `nself build` recorded for the project.
dc() {
  local args=() f
  while IFS= read -r f; do [ -n "$f" ] && args+=(-f "$f"); done <"$project/.nself/compose-files.txt"
  while IFS= read -r f; do [ -n "$f" ] && args+=(--env-file "$f"); done <"$project/.nself/compose-env-files.txt"
  (cd "$project" && "${compose[@]}" "${args[@]}" "$@")
}
project_containers() { docker ps -aq --filter "label=com.docker.compose.project=$proj_name"; }

dump() {
  echo "---- compose state ----" >&2
  docker ps -a --filter "label=com.docker.compose.project=$proj_name" >&2 || true
  for c in $(project_containers); do echo "---- logs $c ----" >&2; docker logs --tail 40 "$c" >&2 || true; done
}
cleanup() {
  rc=$?
  [ "$rc" -ne 0 ] && dump
  if [ -f "$project/.nself/compose-files.txt" ]; then dc down -v --remove-orphans >/dev/null 2>&1 || true; fi
  project_containers | xargs -r docker rm -f >/dev/null 2>&1 || true
  [ "$made_work" = 1 ] && rm -rf "$work"
  exit "$rc"
}
trap cleanup EXIT

# lay_plugin <ref|working-tree>: place the plugin files where `nself add` puts them.
lay_plugin() {
  rm -rf "$work/plugins/forgejo"; mkdir -p "$work/plugins/forgejo"
  if [ "$1" = tree ]; then
    cp "$plugin/docker-compose.plugin.yml" "$plugin/forgejo-bootstrap.sh" "$work/plugins/forgejo/"
  else
    for f in docker-compose.plugin.yml forgejo-bootstrap.sh runner-entrypoint.sh; do
      git -C "$repo" show "$1:free/forgejo/$f" >"$work/plugins/forgejo/$f" 2>/dev/null || rm -f "$work/plugins/forgejo/$f"
    done
    # The previous fragment mounted ./forgejo-bootstrap.sh, which compose resolves against the project directory.
    cp "$work/plugins/forgejo"/*.sh "$project/"
  fi
}
mounts_of_project() { for c in $(project_containers); do docker inspect --format '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' "$c"; done; }
wait_for() { # <description> <command...>
  local what=$1; shift
  for _ in $(seq 1 "$wait_s"); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done
  fail "timed out after ${wait_s}s waiting for $what"
}
forgejo_healthy() { [ "$(docker inspect --format '{{.State.Health.Status}}' "${proj_name}_forgejo" 2>/dev/null)" = healthy ]; }
init_done() { [ "$(docker inspect --format '{{.State.Status}} {{.State.ExitCode}}' "${proj_name}_forgejo_init" 2>/dev/null)" = "exited 0" ]; }

step "scratch nself project"
(cd "$project" && nself init --non-interactive --name "$proj_name" --profile tiny --quiet)
# Throwaway admin credentials for this run only; the password is generated, never a literal.
{
  printf 'NSELF_FORGEJO_ADMIN_USER=smokeadmin\n'
  printf 'NSELF_FORGEJO_ADMIN_PASSWORD=%s\n' "$(head -c 18 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  printf 'NSELF_FORGEJO_ADMIN_EMAIL=smoke@example.invalid\n'
} >>"$project/.env"

upgrade=0
if git -C "$repo" cat-file -e "$base_ref:free/forgejo/runner-entrypoint.sh" 2>/dev/null; then
  upgrade=1
  step "previous fragment ($base_ref) up"
  lay_plugin "$base_ref"
  (cd "$project" && nself build --no-migration-check --quiet)
  dc up -d forgejo forgejo-init forgejo-runner
  wait_for "previous forgejo healthy" forgejo_healthy
  runner="$(docker ps -aq --filter "name=${proj_name}_forgejo_runner")"
  [ -n "$runner" ] || fail "previous stack has no forgejo-runner container: the smoke would prove nothing"
  case "$(mounts_of_project)" in *"$sock"*) ;; *) fail "previous stack does not mount the Docker socket: the smoke would prove nothing" ;; esac
  echo "ok: previous stack runs the runner and mounts the Docker socket"
else
  echo "note: $base_ref has no runner-entrypoint.sh; checking the fresh-install result only"
fi

step "this checkout's fragment: nself build --remove-orphans"
lay_plugin tree
rm -f "$project/forgejo-bootstrap.sh" "$project/runner-entrypoint.sh"
(cd "$project" && nself build --no-migration-check --remove-orphans --quiet)
if [ "$upgrade" = 1 ]; then
  [ -z "$(docker ps -aq --filter "name=${proj_name}_forgejo_runner")" ] || fail "orphan forgejo-runner container survived --remove-orphans"
  echo "ok: the orphan runner container was removed"
fi
dc up -d forgejo forgejo-init
wait_for "forgejo healthy" forgejo_healthy
wait_for "forgejo-init to exit 0" init_done

step "assertions"
services="$(dc config --services | grep -E '^forgejo' | sort | tr '\n' ' ')"
[ "$services" = "forgejo forgejo-init " ] || fail "forgejo services are '$services', want 'forgejo forgejo-init '"
case "$(mounts_of_project)" in *"$sock"*) fail "a container of the project still mounts the Docker socket" ;; esac
[ -z "$(docker volume ls -q --filter "name=${proj_name}_forgejo_runner_config")" ] || echo "note: ${proj_name}_forgejo_runner_config still exists (README migration step: docker volume rm)"
env_of_forgejo="$(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "${proj_name}_forgejo")"
case "$env_of_forgejo" in *GITEA__service__DISABLE_REGISTRATION=true*) ;; *) fail "registration is not closed by default" ;; esac
echo "PASS: no runner, no Docker socket mount, forgejo-init exit 0, registration closed"
