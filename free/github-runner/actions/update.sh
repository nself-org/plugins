#!/bin/bash
# Update GitHub Actions runner to the latest version

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"
source "${SHARED_DIR}/plugin-utils.sh"

RUNNER_DIR="${HOME}/.nself/runners/github-runner"

if [[ ! -f "${RUNNER_DIR}/run.sh" ]]; then
  plugin_error "Runner not installed. Run: nself plugin install github-runner"
  exit 1
fi

# Get current version
current_ver="(unknown)"
if [[ -f "${RUNNER_DIR}/bin/Runner.Listener" ]]; then
  current_ver=$(${RUNNER_DIR}/bin/Runner.Listener --version 2>/dev/null | head -1 || echo "(unknown)")
fi

plugin_info "Current version: $current_ver"
plugin_info "Stopping runner before update..."

# Stop
"${PLUGIN_DIR}/actions/stop.sh" 2>/dev/null || true

# Re-run installer (skip configure since .credentials already exist)
SKIP_CONFIGURE=true "${PLUGIN_DIR}/install.sh"

plugin_success "Runner update complete."
