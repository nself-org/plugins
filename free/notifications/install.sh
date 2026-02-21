#!/bin/bash
# =============================================================================
# Notifications Plugin Installer
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

install_notifications_plugin() {
    plugin_info "Installing Notifications plugin..."

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

            if ! schema_migration_applied "notifications" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "notifications" "$migration_name"
            fi
        done
    fi

    # Create cache and log directories
    mkdir -p "${HOME}/.nself/cache/plugins/notifications"
    mkdir -p "${HOME}/.nself/logs/plugins/notifications"
    mkdir -p "${HOME}/.nself/templates/notifications"

    # Copy default templates
    if [[ -d "${PLUGIN_DIR}/templates" ]]; then
        cp -r "${PLUGIN_DIR}/templates/"* "${HOME}/.nself/templates/notifications/" 2>/dev/null || true
    fi

    plugin_success "Notifications plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Configure email provider in .env (see .env.example)\n"
    printf "  2. Run 'nself plugin notifications test' to verify setup\n"
    printf "  3. Start notification server: 'nself plugin notifications server'\n"
    printf "  4. Start worker for queue processing: 'nself plugin notifications worker'\n"
    printf "\n"
    printf "Available providers:\n"
    printf "  Email: Resend, SendGrid, Mailgun, AWS SES, SMTP\n"
    printf "  Push:  FCM, OneSignal, Web Push\n"
    printf "  SMS:   Twilio, Plivo, AWS SNS\n"
    printf "\n"
}

# Run installation
install_notifications_plugin
