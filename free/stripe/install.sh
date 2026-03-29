#!/bin/bash
# =============================================================================
# Stripe Plugin Installer
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

install_stripe_plugin() {
    plugin_info "Installing Stripe plugin..."

    # Check required environment variables
    if [[ -z "${STRIPE_API_KEY:-}" && -z "${STRIPE_API_KEYS:-}" ]]; then
        plugin_warn "Stripe API key not set. Configure STRIPE_API_KEY or STRIPE_API_KEYS before using the plugin."
        plugin_info "Add to your .env: STRIPE_API_KEY=sk_live_... (or STRIPE_API_KEYS=sk_live_legacy,sk_live_rebrand)"
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

            if ! schema_migration_applied "stripe" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "stripe" "$migration_name"
            fi
        done
    fi

    # Create cache and log directories
    mkdir -p "${HOME}/.nself/cache/plugins/stripe"
    mkdir -p "${HOME}/.nself/logs/plugins/stripe"

    # Register webhook endpoint (if webhook secret is configured)
    if [[ -n "${STRIPE_WEBHOOK_SECRET:-}" ]]; then
        plugin_info "Webhook endpoint configured"
    else
        plugin_info "To enable webhooks, set STRIPE_WEBHOOK_SECRET in your .env"
    fi

    plugin_success "Stripe plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Add STRIPE_API_KEY (or STRIPE_API_KEYS) to your .env file\n"
    printf "  2. Run 'nself plugin stripe sync' to import existing data\n"
    printf "  3. Configure webhooks at https://dashboard.stripe.com/webhooks\n"
    printf "\n"
}

# Run installation
install_stripe_plugin
