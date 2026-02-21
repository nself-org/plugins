#!/bin/bash
# =============================================================================
# Jobs Plugin Installer
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"
source "${SHARED_DIR}/schema-sync.sh"

# =============================================================================
# Installation
# =============================================================================

install_jobs_plugin() {
    plugin_info "Installing Jobs plugin..."

    # Check required environment variables
    if ! plugin_check_env "jobs" "JOBS_REDIS_URL"; then
        plugin_error "JOBS_REDIS_URL is required for the Jobs plugin"
        plugin_info "Add to your .env: JOBS_REDIS_URL=redis://localhost:6379"
        return 1
    fi

    # Validate Redis connection
    plugin_info "Checking Redis connection..."
    if command -v redis-cli &> /dev/null; then
        local redis_url="${JOBS_REDIS_URL}"
        local redis_host=$(echo "$redis_url" | sed -E 's|redis://([^:]+).*|\1|')
        local redis_port=$(echo "$redis_url" | sed -E 's|redis://[^:]+:([0-9]+).*|\1|')

        if redis-cli -h "${redis_host:-localhost}" -p "${redis_port:-6379}" ping &>/dev/null; then
            plugin_success "Redis connection successful"
        else
            plugin_warn "Cannot connect to Redis at ${redis_url}"
            plugin_info "Make sure Redis is running before starting workers"
        fi
    else
        plugin_warn "redis-cli not found, skipping connection check"
    fi

    # Apply database schema
    plugin_info "Applying database schema..."

    # Ensure migrations table exists
    schema_ensure_migrations_table

    # Apply main schema
    if [[ -f "${PLUGIN_DIR}/schema/tables.sql" ]]; then
        plugin_db_exec_file "${PLUGIN_DIR}/schema/tables.sql"
    fi

    # Apply migrations
    if [[ -d "${PLUGIN_DIR}/schema/migrations" ]]; then
        for migration in "${PLUGIN_DIR}/schema/migrations"/*.sql; do
            [[ ! -f "$migration" ]] && continue

            local migration_name
            migration_name=$(basename "$migration" .sql)

            if ! schema_migration_applied "jobs" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "jobs" "$migration_name"
            fi
        done
    fi

    # Create cache and log directories
    mkdir -p "${HOME}/.nself/cache/plugins/jobs"
    mkdir -p "${HOME}/.nself/logs/plugins/jobs"

    # Install TypeScript dependencies if package.json exists
    if [[ -f "${PLUGIN_DIR}/ts/package.json" ]]; then
        plugin_info "Installing dependencies..."
        (cd "${PLUGIN_DIR}/ts" && npm install --silent)

        plugin_info "Building TypeScript..."
        (cd "${PLUGIN_DIR}/ts" && npm run build --silent)
    fi

    plugin_success "Jobs plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Ensure Redis is running at: %s\n" "${JOBS_REDIS_URL}"
    printf "  2. Start the BullBoard dashboard: nself plugin jobs server\n"
    printf "  3. Start a worker process: nself plugin jobs worker\n"
    printf "  4. View job stats: nself plugin jobs stats\n"
    printf "\n"
    printf "Configuration:\n"
    printf "  Dashboard: http://localhost:%s%s\n" \
        "${JOBS_DASHBOARD_PORT:-3105}" \
        "${JOBS_DASHBOARD_PATH:-/dashboard}"
    printf "  Default concurrency: %s\n" "${JOBS_DEFAULT_CONCURRENCY:-5}"
    printf "  Max retries: %s\n" "${JOBS_RETRY_ATTEMPTS:-3}"
    printf "\n"
}

# Run installation
install_jobs_plugin
