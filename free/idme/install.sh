#!/bin/bash
# =============================================================================
# ID.me Plugin Installer
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

install_idme_plugin() {
    plugin_info "Installing ID.me plugin..."

    # Check required environment variables
    if ! plugin_check_env "idme" "IDME_CLIENT_ID"; then
        plugin_warn "IDME_CLIENT_ID not set. You'll need to configure it before using the plugin."
        plugin_info "Add to your .env: IDME_CLIENT_ID=your_client_id"
    fi

    if ! plugin_check_env "idme" "IDME_CLIENT_SECRET"; then
        plugin_warn "IDME_CLIENT_SECRET not set. You'll need to configure it before using the plugin."
        plugin_info "Add to your .env: IDME_CLIENT_SECRET=your_client_secret"
    fi

    if ! plugin_check_env "idme" "IDME_REDIRECT_URI"; then
        plugin_warn "IDME_REDIRECT_URI not set. You'll need to configure it before using the plugin."
        plugin_info "Add to your .env: IDME_REDIRECT_URI=https://your-domain.com/callback/idme"
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

            if ! schema_migration_applied "idme" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "idme" "$migration_name"
            fi
        done
    fi

    # Create cache and log directories
    mkdir -p "${HOME}/.nself/cache/plugins/idme"
    mkdir -p "${HOME}/.nself/logs/plugins/idme"

    # Register webhook endpoint (if webhook secret is configured)
    if [[ -n "${IDME_WEBHOOK_SECRET:-}" ]]; then
        plugin_info "Webhook endpoint configured"
    else
        plugin_info "To enable webhooks, set IDME_WEBHOOK_SECRET in your .env"
    fi

    plugin_success "ID.me plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Register your application at https://developers.id.me\n"
    printf "  2. Add OAuth credentials to your .env file:\n"
    printf "       IDME_CLIENT_ID=your_client_id\n"
    printf "       IDME_CLIENT_SECRET=your_client_secret\n"
    printf "       IDME_REDIRECT_URI=https://your-domain.com/callback/idme\n"
    printf "  3. (Optional) Enable sandbox mode for testing:\n"
    printf "       IDME_SANDBOX=true\n"
    printf "  4. (Optional) Customize scopes:\n"
    printf "       IDME_SCOPES=openid,email,profile,military,veteran\n"
    printf "  5. Test the connection:\n"
    printf "       nself plugin idme test\n"
    printf "\n"
}

# Run installation
install_idme_plugin
