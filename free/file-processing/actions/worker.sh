#!/bin/bash
# =============================================================================
# File Processing - Start Worker
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

cd "${PLUGIN_DIR}/ts"

# Check if built
if [[ ! -d "dist" ]]; then
    plugin_error "Plugin not built. Run: nself plugin file-processing init"
    exit 1
fi

# Load environment
if [[ -f ".env" ]]; then
    set -a
    source .env
    set +a
fi

CONCURRENCY="${FILE_QUEUE_CONCURRENCY:-3}"

plugin_info "Starting File Processing worker (concurrency: ${CONCURRENCY})..."
plugin_info "Press Ctrl+C to stop"

# Start worker
node dist/worker.js
