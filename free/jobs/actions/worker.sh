#!/bin/bash
# =============================================================================
# Jobs Worker Action
# Start job worker process
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Worker
# =============================================================================

start_worker() {
    local queue="${1:-default}"
    local concurrency="${JOBS_DEFAULT_CONCURRENCY:-5}"

    plugin_info "Starting worker for queue: $queue (concurrency: $concurrency)..."

    # Check if TypeScript build exists
    if [[ ! -f "${PLUGIN_DIR}/ts/dist/worker.js" ]]; then
        plugin_error "Worker not built. Run 'cd ${PLUGIN_DIR}/ts && npm run build'"
        return 1
    fi

    # Check Redis connection
    if ! command -v redis-cli &> /dev/null; then
        plugin_warn "redis-cli not found, skipping connection check"
    else
        local redis_url="${JOBS_REDIS_URL:-redis://localhost:6379}"
        local redis_host=$(echo "$redis_url" | sed -E 's|redis://([^:]+).*|\1|')
        local redis_port=$(echo "$redis_url" | sed -E 's|redis://[^:]+:([0-9]+).*|\1|')

        if ! redis-cli -h "${redis_host:-localhost}" -p "${redis_port:-6379}" ping &>/dev/null; then
            plugin_error "Cannot connect to Redis at ${redis_url}"
            return 1
        fi
    fi

    printf "\n"
    plugin_success "Worker started for queue '$queue'"
    printf "\n"

    # Start worker
    export WORKER_QUEUE="$queue"
    export WORKER_CONCURRENCY="$concurrency"
    cd "${PLUGIN_DIR}/ts" && node dist/worker.js
}

# Show help
show_help() {
    printf "Usage: nself plugin jobs worker [QUEUE] [OPTIONS]\n\n"
    printf "Start a job worker process.\n\n"
    printf "Arguments:\n"
    printf "  QUEUE                Queue name (default: default)\n\n"
    printf "Options:\n"
    printf "  -c, --concurrency N  Worker concurrency (default: 5)\n"
    printf "  -h, --help           Show this help\n\n"
    printf "Environment:\n"
    printf "  JOBS_REDIS_URL            Redis connection URL (required)\n"
    printf "  JOBS_DEFAULT_CONCURRENCY  Default concurrency (default: 5)\n"
    printf "  JOBS_JOB_TIMEOUT          Job timeout in ms (default: 60000)\n"
    printf "  JOBS_RETRY_ATTEMPTS       Max retry attempts (default: 3)\n\n"
    printf "Examples:\n"
    printf "  nself plugin jobs worker                 # Start worker for 'default' queue\n"
    printf "  nself plugin jobs worker high-priority   # Start worker for 'high-priority' queue\n"
    printf "  JOBS_DEFAULT_CONCURRENCY=10 nself plugin jobs worker\n"
}

# Parse arguments
QUEUE="default"

while [[ $# -gt 0 ]]; do
    case "$1" in
        -c|--concurrency)
            export JOBS_DEFAULT_CONCURRENCY="$2"
            shift 2
            ;;
        -h|--help|help)
            show_help
            exit 0
            ;;
        *)
            QUEUE="$1"
            shift
            ;;
    esac
done

start_worker "$QUEUE"
