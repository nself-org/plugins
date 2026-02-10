#!/bin/bash
# =============================================================================
# Donorbox Plugin Installer
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

install_donorbox_plugin() {
    plugin_info "Installing Donorbox plugin..."

    # Check required environment variables
    if [[ -z "${DONORBOX_EMAIL:-}" && -z "${DONORBOX_EMAILS:-}" ]]; then
        plugin_warn "Donorbox credentials not set. Configure DONORBOX_EMAIL/DONORBOX_API_KEY or DONORBOX_EMAILS/DONORBOX_API_KEYS before using the plugin."
        plugin_info "Add to your .env: DONORBOX_EMAIL=... DONORBOX_API_KEY=..."
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

            if ! schema_migration_applied "donorbox" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "donorbox" "$migration_name"
            fi
        done
    fi

    # Create cache and log directories
    mkdir -p "${HOME}/.nself/cache/plugins/donorbox"
    mkdir -p "${HOME}/.nself/logs/plugins/donorbox"

    # Webhook setup info
    if [[ -n "${DONORBOX_WEBHOOK_SECRET:-}" ]]; then
        plugin_info "Webhook secret configured"
    else
        plugin_info "To enable webhooks, set DONORBOX_WEBHOOK_SECRET in your .env"
    fi

    plugin_success "Donorbox plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Add DONORBOX_EMAIL and DONORBOX_API_KEY to your .env file\n"
    printf "  2. Run 'nself plugin donorbox sync' to import existing data\n"
    printf "  3. Configure webhooks in your Donorbox dashboard\n"
    printf "\n"
}

# Run installation
install_donorbox_plugin
