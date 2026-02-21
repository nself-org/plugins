#!/bin/bash
# =============================================================================
# Jobs Server Action
# Start BullBoard dashboard server
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Server
# =============================================================================

start_server() {
    local port="${JOBS_DASHBOARD_PORT:-3105}"
    local path="${JOBS_DASHBOARD_PATH:-/dashboard}"

    plugin_info "Starting BullBoard dashboard server..."

    # Check if TypeScript build exists
    if [[ ! -f "${PLUGIN_DIR}/ts/dist/server.js" ]]; then
        plugin_error "Server not built. Run 'cd ${PLUGIN_DIR}/ts && npm run build'"
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
    plugin_success "Dashboard will be available at: http://localhost:${port}${path}"
    printf "\n"

    # Start server
    cd "${PLUGIN_DIR}/ts" && node dist/server.js
}

# Show help
show_help() {
    printf "Usage: nself plugin jobs server [OPTIONS]\n\n"
    printf "Start the BullBoard dashboard server.\n\n"
    printf "Options:\n"
    printf "  -h, --help           Show this help\n\n"
    printf "Environment:\n"
    printf "  JOBS_REDIS_URL            Redis connection URL (required)\n"
    printf "  JOBS_DASHBOARD_PORT       Dashboard port (default: 3105)\n"
    printf "  JOBS_DASHBOARD_PATH       Dashboard path (default: /dashboard)\n"
    printf "  JOBS_DASHBOARD_ENABLED    Enable dashboard (default: true)\n\n"
    printf "Example:\n"
    printf "  nself plugin jobs server\n"
    printf "  JOBS_DASHBOARD_PORT=4000 nself plugin jobs server\n"
}

# Parse arguments
case "${1:-}" in
    -h|--help|help)
        show_help
        ;;
    *)
        start_server
        ;;
esac
