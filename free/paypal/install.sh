#!/bin/bash
# =============================================================================
# PayPal Plugin Installer
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

install_paypal_plugin() {
    plugin_info "Installing PayPal plugin..."

    # Check required environment variables
    if [[ -z "${PAYPAL_CLIENT_ID:-}" && -z "${PAYPAL_CLIENT_IDS:-}" ]]; then
        plugin_warn "PayPal client credentials not set. Configure PAYPAL_CLIENT_ID/PAYPAL_CLIENT_SECRET or PAYPAL_CLIENT_IDS/PAYPAL_CLIENT_SECRETS before using the plugin."
        plugin_info "Add to your .env: PAYPAL_CLIENT_ID=... PAYPAL_CLIENT_SECRET=..."
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

            if ! schema_migration_applied "paypal" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "paypal" "$migration_name"
            fi
        done
    fi

    # Create cache and log directories
    mkdir -p "${HOME}/.nself/cache/plugins/paypal"
    mkdir -p "${HOME}/.nself/logs/plugins/paypal"

    # Webhook setup info
    if [[ -n "${PAYPAL_WEBHOOK_ID:-}" ]]; then
        plugin_info "Webhook ID configured"
    else
        plugin_info "To enable webhooks, set PAYPAL_WEBHOOK_ID in your .env"
        plugin_info "Create a webhook at https://developer.paypal.com/dashboard/webhooks"
    fi

    plugin_success "PayPal plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Add PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET to your .env file\n"
    printf "  2. Run 'nself plugin paypal sync' to import existing data\n"
    printf "  3. Configure webhooks at https://developer.paypal.com/dashboard/webhooks\n"
    printf "\n"
}

# Run installation
install_paypal_plugin
