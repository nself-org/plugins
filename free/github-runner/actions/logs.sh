#!/bin/bash
# Tail GitHub Actions runner logs

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"
source "${SHARED_DIR}/plugin-utils.sh"

LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
RUNNER_DIR="${HOME}/.nself/runners/github-runner"

# systemd logs
if command -v journalctl &>/dev/null; then
  svc_name="actions.runner.${GITHUB_RUNNER_ORG:-nself-org}.${GITHUB_RUNNER_NAME:-$(hostname)-nself}"
  if systemctl is-enabled --quiet "$svc_name" 2>/dev/null; then
    plugin_info "Streaming systemd journal for $svc_name..."
    journalctl -u "$svc_name" -n 50 -f
    exit 0
  fi
fi

# nohup log
LOG_FILE="${LOG_DIR}/runner.log"
if [[ -f "$LOG_FILE" ]]; then
  plugin_info "Streaming $LOG_FILE..."
  tail -n 50 -f "$LOG_FILE"
else
  plugin_warn "No log file found at $LOG_FILE"
  plugin_info "Try: nself plugin github-runner start"
fi
