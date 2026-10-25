#!/bin/bash
# =============================================================================
# Trusted-only guard for the GitHub Actions runner (Epic D23, ADR 0025 s2)
# =============================================================================
# Sourced by install.sh, actions/start.sh and actions/status.sh. A persistent
# runner on the nSelf host must only run jobs from repositories the operator
# trusts, so before registration, on every start and in status the runner group
# (org scope) or repository (repo scope) is read through the GitHub API and one
# that is open to public repositories is refused with `runner_group_public`.
#
# Needs: curl, jq. The PAT goes to curl through a config on stdin, never argv.
# GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true accepts the risk (warned on every use).
# =============================================================================

TRUST_API_BASE="https://api.github.com"
TRUST_GROUP_PAGE_SIZE=100
TRUST_MAX_PAGES=50

# trust_api_get URL: print the JSON body of an authenticated GET; fail if the
# call or the status is not a 200. Error text never contains the PAT.
trust_api_get() {
  local url="$1" out status body
  out=$(printf 'header = "Authorization: Bearer %s"\n' "${GITHUB_RUNNER_PAT}" \
    | curl -sS -K - -w '\n%{http_code}' \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "$url" 2>/dev/null) || return 1
  status="${out##*$'\n'}"
  body="${out%$'\n'*}"
  [[ "$status" == "200" ]] || return 1
  printf '%s' "$body"
}

# trust_unverifiable MESSAGE: the check could not be completed. Fails closed.
trust_unverifiable() {
  printf 'runner_trust_unverifiable: %s\n' "$1" >&2
  return 2
}

# trust_scope: print the normalised scope (org or repo, case-insensitive,
# default org); fail on anything else so no caller guesses a scope.
trust_scope() {
  local s
  s=$(printf '%s' "${GITHUB_RUNNER_SCOPE:-org}" | tr '[:upper:]' '[:lower:]')
  [[ -n "$s" ]] || s=org
  case "$s" in
    org|repo) printf '%s' "$s" ;;
    *) return 1 ;;
  esac
}

# trust_valid_name VALUE: GitHub owner and repo names only.
trust_valid_name() { [[ "$1" =~ ^[A-Za-z0-9._-]+$ ]]; }

# trust_check: 0 = trusted (or override set), 1 = refused (runner_group_public),
# 2 = unverifiable (fail closed), 3 = group not found. Sets TRUST_GROUP.
trust_check() {
  local org="${GITHUB_RUNNER_ORG:-}" scope
  local allow="${GITHUB_RUNNER_ALLOW_PUBLIC_REPOS:-false}"
  TRUST_GROUP=""
  if [[ -z "${GITHUB_RUNNER_PAT:-}" || -z "$org" ]]; then
    trust_unverifiable "GITHUB_RUNNER_PAT and GITHUB_RUNNER_ORG must be set so the runner group can be checked"
    return 2
  fi
  command -v jq >/dev/null 2>&1 || { trust_unverifiable "jq is required for the runner group check"; return 2; }
  scope=$(trust_scope) || { trust_unverifiable "GITHUB_RUNNER_SCOPE must be org or repo (got \"${GITHUB_RUNNER_SCOPE}\")"; return 2; }
  trust_valid_name "$org" || { trust_unverifiable "GITHUB_RUNNER_ORG contains characters GitHub does not allow"; return 2; }
  if [[ "$scope" == "repo" && -n "${GITHUB_RUNNER_REPO:-}" ]]; then
    trust_valid_name "$GITHUB_RUNNER_REPO" || { trust_unverifiable "GITHUB_RUNNER_REPO contains characters GitHub does not allow"; return 2; }
  fi
  if [[ "$scope" == "repo" ]]; then
    _trust_check_repo "$org" "${GITHUB_RUNNER_REPO:-}" "$allow"
  else
    _trust_check_group "$org" "${GITHUB_RUNNER_GROUP:-}" "$allow"
  fi
}

_trust_check_group() {
  local org="$1" group="$2" allow="$3" page body found public count
  for ((page = 1; page <= TRUST_MAX_PAGES; page++)); do
    body=$(trust_api_get "${TRUST_API_BASE}/orgs/${org}/actions/runner-groups?per_page=${TRUST_GROUP_PAGE_SIZE}&page=${page}") \
      || { trust_unverifiable "GitHub API call for the runner groups failed (token needs the organisation \"Self-hosted runners\" permission)"; return 2; }
    found=$(printf '%s' "$body" | jq -r --arg g "$group" '
      [.runner_groups[]? | select(if $g == "" then .default == true else (.name | ascii_downcase) == ($g | ascii_downcase) end)]
      | first | if . == null then "" else "\(.name)\t\(.allows_public_repositories)" end' 2>/dev/null) \
      || { trust_unverifiable "could not decode the runner groups response"; return 2; }
    [[ -n "$found" ]] && break
    count=$(printf '%s' "$body" | jq '.runner_groups | length' 2>/dev/null) || count=0
    [[ "${count:-0}" =~ ^[0-9]+$ && "${count:-0}" -lt "$TRUST_GROUP_PAGE_SIZE" ]] && break
  done
  if [[ -z "$found" ]]; then
    printf 'runner_group_not_found: runner group "%s" not found in the organisation\n' "${group:-<default>}" >&2
    return 3
  fi
  TRUST_GROUP="${found%%$'\t'*}"
  public="${found##*$'\t'}"
  case "$public" in
    false) return 0 ;;
    true) ;;
    *) trust_unverifiable "GitHub did not report whether runner group \"${TRUST_GROUP}\" allows public repositories"; return 2 ;;
  esac
  _trust_public "runner group \"${TRUST_GROUP}\" is open to public repositories" \
    "Restrict the group to private repositories in the organisation settings" "$allow"
}

_trust_check_repo() {
  local org="$1" repo="$2" allow="$3" body private
  if [[ -z "$repo" ]]; then
    trust_unverifiable "GITHUB_RUNNER_REPO is required for repo scope"
    return 2
  fi
  body=$(trust_api_get "${TRUST_API_BASE}/repos/${org}/${repo}") \
    || { trust_unverifiable "GitHub API call for the repository failed"; return 2; }
  private=$(printf '%s' "$body" | jq -r 'if .private == null then "unknown" else (.private | tostring) end' 2>/dev/null) \
    || { trust_unverifiable "could not decode the repository response"; return 2; }
  case "$private" in
    true) return 0 ;;
    false) _trust_public "repository ${org}/${repo} is public" "Use a private repository" "$allow" ;;
    *) trust_unverifiable "GitHub did not report whether the repository is private"; return 2 ;;
  esac
}

# _trust_public WHAT FIX ALLOW: refuse (1) unless the override is set (0, warn).
_trust_public() {
  local what="$1" fix="$2" allow="$3"
  if [[ "$allow" == "true" ]]; then
    printf 'WARNING: %s; GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true lets fork pull requests run on this host\n' "$what" >&2
    return 0
  fi
  printf 'runner_group_public: %s, so fork pull requests could run on this host. %s, or set GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true to accept the risk\n' \
    "$what" "$fix" >&2
  return 1
}

# trust_deploy_host_warning: print the warning when ENV is prod, production or
# staging, else nothing.
trust_deploy_host_warning() {
  case "$(printf '%s' "${ENV:-}" | tr '[:upper:]' '[:lower:]')" in
    prod|production|staging)
      printf '%s\n' "this runner shares a host with a deployed nSelf stack: it is a persistent, non-isolated runner. Use it for trusted private repositories only; never for fork pull requests"
      ;;
  esac
}

# trust_service_name: the systemd unit of the installed runner. svc.sh writes
# the exact name to RUNNER_DIR/.service (it differs for repo scope and long
# names); fall back to the org-scope pattern.
trust_service_name() {
  local f="${RUNNER_DIR:-}/.service"
  if [[ -n "${RUNNER_DIR:-}" && -s "$f" ]]; then
    tr -d '[:space:]' < "$f"
    return
  fi
  printf 'actions.runner.%s.%s' "${GITHUB_RUNNER_ORG:-nself-org}" "${GITHUB_RUNNER_NAME:-$(hostname)-nself}"
}

# TRUST_STOP_GRACE: seconds a stop waits after TERM before it escalates to KILL
# (and again after KILL before it reports survivors).
TRUST_STOP_GRACE="${TRUST_STOP_GRACE:-5}"

# _trust_tree_pids PID: print PID and all its descendants, one per line,
# descendants first. run.sh starts Runner.Listener and does not forward
# signals, so killing only the wrapper leaves a listener that still takes jobs.
_trust_tree_pids() {
  local pid="$1" child
  for child in $(pgrep -P "$pid" 2>/dev/null || true); do
    _trust_tree_pids "$child"
  done
  printf '%s\n' "$pid"
}

# _trust_pid_alive PID: running (a zombie awaiting its parent counts as gone).
_trust_pid_alive() {
  local state
  state=$(ps -o stat= -p "$1" 2>/dev/null | tr -d ' ') || return 1
  [[ -n "$state" && "$state" != Z* ]]
}

# _trust_live_pids PID...: print, space separated, those PIDs still running.
_trust_live_pids() {
  local p out=""
  for p in "$@"; do
    ! _trust_pid_alive "$p" || out="${out:+$out }$p"
  done
  printf '%s' "$out"
}

# _trust_kill_tree PID: stop the process and all its descendants. The tree is
# listed once, before any signal, so a child that outlives a dead wrapper (and
# is reparented) is still signalled: TERM to all, a grace wait, then KILL to
# the survivors of that same list. Returns 1 if a process is still running.
_trust_kill_tree() {
  local pid="$1" live waited p
  live=$(_trust_tree_pids "$pid" | tr '\n' ' ')
  for p in $live; do kill -TERM "$p" 2>/dev/null || true; done
  for ((waited = 0; waited < TRUST_STOP_GRACE * 5; waited++)); do
    live=$(_trust_live_pids $live)
    [[ -n "$live" ]] || return 0
    sleep 0.2
  done
  for p in $live; do kill -KILL "$p" 2>/dev/null || true; done
  for ((waited = 0; waited < TRUST_STOP_GRACE * 5; waited++)); do
    live=$(_trust_live_pids $live)
    [[ -n "$live" ]] || return 0
    sleep 0.2
  done
  printf 'runner process(es) still running after KILL: %s\n' "$live" >&2
  return 1
}

# trust_stop_runner PID_FILE SERVICE: stop a running runner (systemd service or
# nohup process tree). The systemd unit is also disabled so a reboot does not
# bring the runner back into a public group; start.sh enables and starts it
# again after the guard passes. Prints what it stopped; nothing when nothing was
# running. Returns non-zero (and says why on stderr) if the service could not
# be stopped or disabled, or a process survived; it still tries every step.
trust_stop_runner() {
  local pid_file="$1" svc="$2" pid sc="" rc=0
  if command -v systemctl >/dev/null 2>&1 && { systemctl is-active --quiet "$svc" 2>/dev/null || systemctl is-enabled --quiet "$svc" 2>/dev/null; }; then
    [[ $EUID -eq 0 ]] || sc="sudo"
    if systemctl is-active --quiet "$svc" 2>/dev/null; then
      if $sc systemctl stop "$svc"; then
        printf 'stopped systemd service %s\n' "$svc"
      else
        printf 'could not stop systemd service %s\n' "$svc" >&2
        rc=1
      fi
    fi
    if $sc systemctl disable "$svc" >/dev/null 2>&1; then
      printf 'disabled systemd service %s (no restart at boot while the group is public)\n' "$svc"
    else
      printf 'could not disable systemd service %s\n' "$svc" >&2
      rc=1
    fi
  fi
  if [[ -f "$pid_file" ]]; then
    pid=$(cat "$pid_file")
    if kill -0 "$pid" 2>/dev/null; then
      if _trust_kill_tree "$pid"; then
        printf 'stopped runner (PID %s and its children)\n' "$pid"
      else
        printf 'could not stop runner (PID %s and its children)\n' "$pid" >&2
        rc=1
      fi
    fi
  fi
  return "$rc"
}
