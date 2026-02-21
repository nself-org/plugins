#!/bin/bash
# =============================================================================
# Notifications Worker Action
# Background worker for processing notification queue
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Worker
# =============================================================================

start_worker() {
    local concurrency="${WORKER_CONCURRENCY:-5}"
    local poll_interval="${WORKER_POLL_INTERVAL:-1000}"

    plugin_info "Starting Notifications worker..."
    plugin_info "Concurrency: $concurrency"
    plugin_info "Poll interval: ${poll_interval}ms"
    printf "\n"

    # Check if TypeScript implementation exists
    if [[ -f "${PLUGIN_DIR}/ts/dist/worker.js" ]]; then
        cd "${PLUGIN_DIR}/ts"
        node dist/worker.js
    elif [[ -f "${PLUGIN_DIR}/ts/src/worker.ts" ]]; then
        cd "${PLUGIN_DIR}/ts"
        if command -v tsx >/dev/null 2>&1; then
            tsx src/worker.ts
        elif command -v ts-node >/dev/null 2>&1; then
            ts-node src/worker.ts
        else
            plugin_error "TypeScript runtime not found (tsx or ts-node required)"
            printf "\nInstall with: npm install -g tsx\n"
            return 1
        fi
    else
        plugin_error "Worker implementation not found"
        printf "\n"
        printf "Build the TypeScript implementation:\n"
        printf "  cd %s/ts\n" "$PLUGIN_DIR"
        printf "  npm install\n"
        printf "  npm run build\n"
        printf "\n"
        return 1
    fi
}

# Show help
show_help() {
    printf "Usage: nself plugin notifications worker [options]\n\n"
    printf "Start the background worker for processing notification queue.\n\n"
    printf "Options:\n"
    printf "  --concurrency N    Number of concurrent workers (default: 5)\n"
    printf "  --poll-interval N  Poll interval in ms (default: 1000)\n\n"
    printf "Environment:\n"
    printf "  WORKER_CONCURRENCY      Number of concurrent workers\n"
    printf "  WORKER_POLL_INTERVAL    Poll interval in milliseconds\n"
    printf "  DATABASE_URL            PostgreSQL connection string\n\n"
    printf "The worker:\n"
    printf "  - Polls notification_queue for pending items\n"
    printf "  - Processes notifications via configured providers\n"
    printf "  - Handles retries with exponential backoff\n"
    printf "  - Updates status in real-time\n"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help|help)
            show_help
            exit 0
            ;;
        --concurrency)
            export WORKER_CONCURRENCY="$2"
            shift 2
            ;;
        --poll-interval)
            export WORKER_POLL_INTERVAL="$2"
            shift 2
            ;;
        *)
            plugin_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

start_worker
