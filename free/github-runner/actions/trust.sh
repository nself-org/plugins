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

# trust_check: 0 = trusted (or override set), 1 = refused (runner_group_public),
# 2 = unverifiable (fail closed), 3 = group not found. Sets TRUST_GROUP.
trust_check() {
  local org="${GITHUB_RUNNER_ORG:-}" scope="${GITHUB_RUNNER_SCOPE:-org}"
  local allow="${GITHUB_RUNNER_ALLOW_PUBLIC_REPOS:-false}"
  TRUST_GROUP=""
  if [[ -z "${GITHUB_RUNNER_PAT:-}" || -z "$org" ]]; then
    trust_unverifiable "GITHUB_RUNNER_PAT and GITHUB_RUNNER_ORG must be set so the runner group can be checked"
    return 2
  fi
  command -v jq >/dev/null 2>&1 || { trust_unverifiable "jq is required for the runner group check"; return 2; }
  if [[ "$scope" == "repo" ]]; then
    _trust_check_repo "$org" "${GITHUB_RUNNER_REPO:-}" "$allow"
  else
    _trust_check_group "$org" "${GITHUB_RUNNER_GROUP:-}" "$allow"
  fi
}

_trust_check_group() {
  local org="$1" group="$2" allow="$3" page body found public
  for ((page = 1; page <= TRUST_MAX_PAGES; page++)); do
    body=$(trust_api_get "${TRUST_API_BASE}/orgs/${org}/actions/runner-groups?per_page=${TRUST_GROUP_PAGE_SIZE}&page=${page}") \
      || { trust_unverifiable "GitHub API call for the runner groups failed (token needs the organisation \"Self-hosted runners\" permission)"; return 2; }
    found=$(printf '%s' "$body" | jq -r --arg g "$group" '
      [.runner_groups[]? | select(if $g == "" then .default == true else (.name | ascii_downcase) == ($g | ascii_downcase) end)]
      | first | if . == null then "" else "\(.name)\t\(.allows_public_repositories)" end' 2>/dev/null) \
      || { trust_unverifiable "could not decode the runner groups response"; return 2; }
    [[ -n "$found" ]] && break
    [[ "$(printf '%s' "$body" | jq '.runner_groups | length' 2>/dev/null)" -lt "$TRUST_GROUP_PAGE_SIZE" ]] && break
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

# trust_stop_runner PID_FILE SERVICE: stop a running runner (systemd service or
# nohup PID). Prints what it stopped; prints nothing when nothing was running.
trust_stop_runner() {
  local pid_file="$1" svc="$2" pid
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$svc" 2>/dev/null; then
    if [[ $EUID -eq 0 ]]; then systemctl stop "$svc"; else sudo systemctl stop "$svc"; fi
    printf 'stopped systemd service %s\n' "$svc"
  fi
  if [[ -f "$pid_file" ]]; then
    pid=$(cat "$pid_file")
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      printf 'stopped runner (PID %s)\n' "$pid"
    fi
  fi
}
