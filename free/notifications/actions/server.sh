#!/bin/bash
# =============================================================================
# Notifications Server Action
# Start HTTP/GraphQL server for notifications
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Server
# =============================================================================

start_server() {
    local port="${PORT:-3102}"
    local host="${HOST:-0.0.0.0}"

    plugin_info "Starting Notifications server..."
    plugin_info "Port: $port"
    plugin_info "Host: $host"
    printf "\n"

    # Check if TypeScript implementation exists
    if [[ -f "${PLUGIN_DIR}/ts/dist/server.js" ]]; then
        cd "${PLUGIN_DIR}/ts"
        node dist/server.js
    elif [[ -f "${PLUGIN_DIR}/ts/src/server.ts" ]]; then
        cd "${PLUGIN_DIR}/ts"
        if command -v tsx >/dev/null 2>&1; then
            tsx src/server.ts
        elif command -v ts-node >/dev/null 2>&1; then
            ts-node src/server.ts
        else
            plugin_error "TypeScript runtime not found (tsx or ts-node required)"
            printf "\nInstall with: npm install -g tsx\n"
            return 1
        fi
    else
        plugin_error "Server implementation not found"
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
    printf "Usage: nself plugin notifications server [options]\n\n"
    printf "Start the notification HTTP server.\n\n"
    printf "Options:\n"
    printf "  --port PORT    Server port (default: 3102)\n"
    printf "  --host HOST    Server host (default: 0.0.0.0)\n\n"
    printf "Environment:\n"
    printf "  PORT           Server port\n"
    printf "  HOST           Server host\n"
    printf "  DATABASE_URL   PostgreSQL connection string\n\n"
    printf "Endpoints:\n"
    printf "  GET  /health                    Health check\n"
    printf "  POST /api/notifications/send    Send notification\n"
    printf "  GET  /api/notifications/:id     Get notification status\n"
    printf "  GET  /api/templates             List templates\n"
    printf "  POST /api/preferences           Update user preferences\n"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help|help)
            show_help
            exit 0
            ;;
        --port)
            export PORT="$2"
            shift 2
            ;;
        --host)
            export HOST="$2"
            shift 2
            ;;
        *)
            plugin_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

start_server
