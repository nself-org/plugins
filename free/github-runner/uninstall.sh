#!/bin/bash
# =============================================================================
# GitHub Actions Runner Plugin Uninstaller
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"

source "${SHARED_DIR}/plugin-utils.sh"

RUNNER_DIR="${HOME}/.nself/runners/github-runner"
LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
PID_FILE="${LOG_DIR}/runner.pid"

# =============================================================================
# Helpers
# =============================================================================

get_removal_token() {
  local scope="${GITHUB_RUNNER_SCOPE:-org}"
  local token

  if [[ "$scope" == "org" ]]; then
    token=$(curl -sSf -X POST \
      -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/orgs/${GITHUB_RUNNER_ORG}/actions/runners/remove-token" \
      | grep '"token"' | head -1 | sed 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  else
    local repo="${GITHUB_RUNNER_REPO:-}"
    token=$(curl -sSf -X POST \
      -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${GITHUB_RUNNER_ORG}/${repo}/actions/runners/remove-token" \
      | grep '"token"' | head -1 | sed 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  fi

  echo "$token"
}

stop_runner_service() {
  # Try systemd first
  if command -v systemctl &>/dev/null; then
    local svc_name="actions.runner.${GITHUB_RUNNER_ORG:-nself-org}.${GITHUB_RUNNER_NAME:-$(hostname)-nself}"
    if systemctl is-active --quiet "$svc_name" 2>/dev/null; then
      plugin_info "Stopping systemd service..."
      if [[ $EUID -eq 0 ]]; then
        systemctl stop "$svc_name" 2>/dev/null || true
      else
        sudo systemctl stop "$svc_name" 2>/dev/null || true
      fi
    fi

    if [[ -f "${RUNNER_DIR}/svc.sh" ]]; then
      (
        cd "$RUNNER_DIR"
        if [[ $EUID -eq 0 ]]; then
          ./svc.sh stop 2>/dev/null || true
          ./svc.sh uninstall 2>/dev/null || true
        else
          sudo ./svc.sh stop 2>/dev/null || true
          sudo ./svc.sh uninstall 2>/dev/null || true
        fi
      )
    fi
  fi

  # Kill nohup process if PID file exists
  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid=$(cat "$PID_FILE")
    if kill -0 "$pid" 2>/dev/null; then
      plugin_info "Stopping runner process (PID: $pid)..."
      kill "$pid" 2>/dev/null || true
      sleep 2
      kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
  fi
}

# =============================================================================
# Uninstallation
# =============================================================================

uninstall_github_runner() {
  plugin_info "Uninstalling GitHub Actions Runner plugin..."

  printf "\n"
  printf "This will:\n"
  printf "  - Stop the runner process\n"
  printf "  - Remove runner registration from GitHub\n"
  printf "  - Delete runner binary and configuration\n"
  printf "\n"
  printf "Continue? (yes/no): "
  read -r confirm
  printf "\n"

  if [[ "$confirm" != "yes" ]]; then
    plugin_warn "Uninstallation cancelled."
    return 0
  fi

  # ------------------------------------------------------------------
  # 1. Stop the runner
  # ------------------------------------------------------------------
  stop_runner_service

  # ------------------------------------------------------------------
  # 2. Remove runner registration from GitHub
  # ------------------------------------------------------------------
  if [[ -n "${GITHUB_RUNNER_PAT:-}" && -n "${GITHUB_RUNNER_ORG:-}" ]]; then
    plugin_info "Removing runner registration from GitHub..."
    local remove_token
    remove_token=$(get_removal_token 2>/dev/null || echo "")

    if [[ -n "$remove_token" && "$remove_token" != "null" && -f "${RUNNER_DIR}/config.sh" ]]; then
      (
        cd "$RUNNER_DIR"
        ./config.sh remove --token "$remove_token" --unattended 2>/dev/null || true
      )
      plugin_success "Runner deregistered from GitHub."
    else
      plugin_warn "Could not get removal token — runner may still appear in GitHub settings."
      plugin_warn "Remove manually at: https://github.com/organizations/${GITHUB_RUNNER_ORG}/settings/actions/runners"
    fi
  else
    plugin_warn "GITHUB_RUNNER_PAT or GITHUB_RUNNER_ORG not set — skipping deregistration."
  fi

  # ------------------------------------------------------------------
  # 3. Remove runner files
  # ------------------------------------------------------------------
  plugin_info "Removing runner files..."
  rm -rf "$RUNNER_DIR"
  rm -rf "$LOG_DIR"

  plugin_success "GitHub Actions Runner plugin uninstalled."
  printf "\n"
}

uninstall_github_runner
