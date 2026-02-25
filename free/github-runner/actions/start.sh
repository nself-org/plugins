#!/bin/bash
# Start the GitHub Actions runner

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"
source "${SHARED_DIR}/plugin-utils.sh"

RUNNER_DIR="${HOME}/.nself/runners/github-runner"
LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
PID_FILE="${LOG_DIR}/runner.pid"

if [[ ! -f "${RUNNER_DIR}/run.sh" ]]; then
  plugin_error "Runner not installed. Run: nself plugin install github-runner"
  exit 1
fi

# Check if already running
if [[ -f "$PID_FILE" ]]; then
  local_pid=$(cat "$PID_FILE")
  if kill -0 "$local_pid" 2>/dev/null; then
    plugin_warn "Runner already running (PID: $local_pid)"
    exit 0
  fi
fi

# Try systemd first
if command -v systemctl &>/dev/null; then
  local svc_name
  svc_name="actions.runner.${GITHUB_RUNNER_ORG:-nself-org}.${GITHUB_RUNNER_NAME:-$(hostname)-nself}"
  if systemctl is-enabled --quiet "$svc_name" 2>/dev/null; then
    plugin_info "Starting systemd service..."
    if [[ $EUID -eq 0 ]]; then
      systemctl start "$svc_name"
    else
      sudo systemctl start "$svc_name"
    fi
    plugin_success "Runner started via systemd."
    exit 0
  fi
fi

# Fallback: nohup
mkdir -p "$LOG_DIR"
plugin_info "Starting runner (nohup)..."
(
  cd "$RUNNER_DIR"
  nohup ./run.sh >> "${LOG_DIR}/runner.log" 2>&1 &
  echo $! > "$PID_FILE"
)
plugin_success "Runner started (PID: $(cat "$PID_FILE"))"
plugin_info "Logs: $LOG_DIR/runner.log"
