#!/bin/bash
# =============================================================================
# File Processing - Cleanup Old Jobs
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# Parse arguments
RETENTION_DAYS="${1:-30}"

plugin_info "Cleaning up jobs older than ${RETENTION_DAYS} days..."

# Call cleanup function
RESULT=$(plugin_db_exec "SELECT cleanup_old_jobs(${RETENTION_DAYS});")

if [[ $? -eq 0 ]]; then
    COUNT=$(echo "${RESULT}" | tail -n 1 | tr -d ' ')
    plugin_success "Cleaned up ${COUNT} old jobs"
else
    plugin_error "Cleanup failed"
    exit 1
fi
