#!/bin/bash
# Stop the GitHub Actions runner

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"
source "${SHARED_DIR}/plugin-utils.sh"
source "${PLUGIN_DIR}/actions/trust.sh"

LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
PID_FILE="${LOG_DIR}/runner.pid"
RUNNER_DIR="${HOME}/.nself/runners/github-runner"

stopped=false
failed=false

# Try systemd first
if command -v systemctl &>/dev/null; then
  svc_name="$(trust_service_name)"
  if systemctl is-active --quiet "$svc_name" 2>/dev/null; then
    plugin_info "Stopping systemd service..."
    if [[ $EUID -eq 0 ]]; then
      systemctl stop "$svc_name"
    else
      sudo systemctl stop "$svc_name"
    fi
    plugin_success "Runner stopped (systemd)."
    stopped=true
  fi
fi

# Kill nohup PID
if [[ -f "$PID_FILE" ]]; then
  pid=$(cat "$PID_FILE")
  if kill -0 "$pid" 2>/dev/null; then
    plugin_info "Stopping runner process (PID: $pid)..."
    if _trust_kill_tree "$pid"; then
      plugin_success "Runner stopped."
      stopped=true
    else
      plugin_error "Runner process did not stop."
      failed=true
    fi
  fi
  [[ "$failed" == "true" ]] || rm -f "$PID_FILE"
fi

if [[ "$failed" == "true" ]]; then
  exit 1
fi

if [[ "$stopped" == "false" ]]; then
  plugin_warn "Runner was not running."
fi
