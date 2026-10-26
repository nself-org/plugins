#!/bin/bash
# Start the GitHub Actions runner

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"
source "${SHARED_DIR}/plugin-utils.sh"
source "${PLUGIN_DIR}/actions/trust.sh"

RUNNER_DIR="${HOME}/.nself/runners/github-runner"
LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
PID_FILE="${LOG_DIR}/runner.pid"

if [[ ! -f "${RUNNER_DIR}/run.sh" ]]; then
  plugin_error "Runner not installed. Run: nself plugin install github-runner"
  exit 1
fi

# Trusted-only guard: re-checked on every start. A group or repo that is now
# open to public repositories stops a running runner unless overridden.
trust_rc=0
trust_check || trust_rc=$?
if [[ "$trust_rc" -ne 0 ]]; then
  if [[ "$trust_rc" -eq 1 || "$trust_rc" -eq 4 ]]; then
    trust_stop_runner "$PID_FILE" "$(trust_service_name)" \
      || plugin_error "The running runner could NOT be fully stopped (see the message above). Stop it manually."
  fi
  plugin_error "Runner not started (see the message above)."
  exit "$trust_rc"
fi
host_warning=$(trust_deploy_host_warning)
[[ -z "$host_warning" ]] || plugin_warn "$host_warning"

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
  svc_name="$(trust_service_name)"
  # `systemctl cat` also finds a unit the trust guard disabled; enable it again
  # now that the guard has passed.
  if systemctl cat "$svc_name" >/dev/null 2>&1; then
    plugin_info "Starting systemd service..."
    if [[ $EUID -eq 0 ]]; then
      systemctl enable --now "$svc_name"
    else
      sudo systemctl enable --now "$svc_name"
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
  RUNNER_ALLOW_RUNASROOT=1 nohup ./run.sh >> "${LOG_DIR}/runner.log" 2>&1 &
  echo $! > "$PID_FILE"
)
plugin_success "Runner started (PID: $(cat "$PID_FILE"))"
plugin_info "Logs: $LOG_DIR/runner.log"
