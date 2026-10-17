#!/bin/bash
# Show GitHub Actions runner status

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"
source "${SHARED_DIR}/plugin-utils.sh"
source "${PLUGIN_DIR}/actions/trust.sh"

RUNNER_DIR="${HOME}/.nself/runners/github-runner"
LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
PID_FILE="${LOG_DIR}/runner.pid"

printf "\n=== GitHub Actions Runner Status ===\n\n"

# ------------------------------------------------------------------
# Trusted-only guard: re-run on every status call. A group or repo that is now
# open to public repositories stops a running runner unless overridden.
# ------------------------------------------------------------------
printf "Trust:\n"
trust_rc=0
trust_check || trust_rc=$?
case "$trust_rc" in
  0) printf "  Check:   passed%s\n" "${TRUST_GROUP:+ (group: $TRUST_GROUP)}" ;;
  1)
    printf "  Check:   REFUSED (runner_group_public)\n"
    trust_stop_runner "$PID_FILE" "$(trust_service_name)" | sed 's/^/  /'
    ;;
  *) printf "  Check:   could not be completed (see the message above)\n" ;;
esac
deploy_host_warning=$(trust_deploy_host_warning)
[[ -z "$deploy_host_warning" ]] || printf "  deploy_host_warning: %s\n" "$deploy_host_warning"
printf "\n"

# ------------------------------------------------------------------
# Local process status
# ------------------------------------------------------------------
printf "Local Process:\n"

is_running=false

# Check systemd
if command -v systemctl &>/dev/null; then
  svc_name="$(trust_service_name)"
  if systemctl is-active --quiet "$svc_name" 2>/dev/null; then
    printf "  Status:  \033[32mRunning\033[0m (systemd: %s)\n" "$svc_name"
    is_running=true
  fi
fi

# Check nohup PID
if [[ "$is_running" == "false" && -f "$PID_FILE" ]]; then
  pid=$(cat "$PID_FILE")
  if kill -0 "$pid" 2>/dev/null; then
    printf "  Status:  \033[32mRunning\033[0m (PID: %s)\n" "$pid"
    is_running=true
  else
    printf "  Status:  \033[31mStopped\033[0m (stale PID file)\n"
  fi
fi

if [[ "$is_running" == "false" ]]; then
  printf "  Status:  \033[31mNot running\033[0m\n"
fi

# ------------------------------------------------------------------
# Runner config
# ------------------------------------------------------------------
if [[ -f "${RUNNER_DIR}/.runner" ]]; then
  printf "\nRunner Config:\n"
  if command -v jq &>/dev/null; then
    jq -r '
      "  Name:    " + .agentName,
      "  URL:     " + .gitHubUrl,
      "  Labels:  " + (.agentLabels | map(.name) | join(","))
    ' "${RUNNER_DIR}/.runner" 2>/dev/null || true
  else
    grep '"agentName"\|"gitHubUrl"' "${RUNNER_DIR}/.runner" | sed 's/.*: "\(.*\)".*/  \1/' || true
  fi
fi

# ------------------------------------------------------------------
# GitHub API status (if PAT available)
# ------------------------------------------------------------------
if [[ -n "${GITHUB_RUNNER_PAT:-}" && -n "${GITHUB_RUNNER_ORG:-}" ]]; then
  printf "\nGitHub Registration:\n"
  local_name="${GITHUB_RUNNER_NAME:-$(hostname)-nself}"

  result=$(curl -sSf \
    -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/orgs/${GITHUB_RUNNER_ORG}/actions/runners" 2>/dev/null || echo '{"runners":[]}')

  if command -v jq &>/dev/null; then
    echo "$result" | jq -r --arg name "$local_name" '
      .runners[] | select(.name == $name) |
      "  Runner:  " + .name + " (ID: " + (.id|tostring) + ")",
      "  Status:  " + .status,
      "  Busy:    " + (.busy|tostring),
      "  Labels:  " + ([.labels[].name] | join(","))
    ' 2>/dev/null || printf "  Not registered (name: %s)\n" "$local_name"
  else
    printf "  (install jq for detailed GitHub status)\n"
  fi
fi

printf "\n"
