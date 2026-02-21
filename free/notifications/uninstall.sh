#!/bin/bash
# =============================================================================
# Notifications Plugin Uninstaller
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Uninstallation
# =============================================================================

uninstall_notifications_plugin() {
    plugin_info "Uninstalling Notifications plugin..."

    # Ask for confirmation
    printf "\n"
    plugin_warn "This will remove all notification data including:"
    printf "  - Notification templates\n"
    printf "  - User preferences\n"
    printf "  - Notification history\n"
    printf "  - Queue items\n"
    printf "  - Provider configurations\n"
    printf "\n"

    printf "Are you sure you want to continue? [y/N] "
    read -r REPLY
    printf "\n"

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        plugin_info "Uninstall cancelled"
        return 0
    fi

    # Drop tables
    plugin_info "Removing database tables..."

    plugin_db_query "DROP TABLE IF EXISTS notification_queue CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS notifications CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS notification_batches CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS notification_providers CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS notification_preferences CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS notification_templates CASCADE;" || true

    # Drop views
    plugin_db_query "DROP VIEW IF EXISTS notification_delivery_rates CASCADE;" || true
    plugin_db_query "DROP VIEW IF EXISTS notification_engagement CASCADE;" || true
    plugin_db_query "DROP VIEW IF EXISTS notification_provider_health CASCADE;" || true
    plugin_db_query "DROP VIEW IF EXISTS notification_user_summary CASCADE;" || true
    plugin_db_query "DROP VIEW IF EXISTS notification_queue_backlog CASCADE;" || true

    # Drop functions
    plugin_db_query "DROP FUNCTION IF EXISTS get_user_notification_preference(UUID, VARCHAR, VARCHAR) CASCADE;" || true
    plugin_db_query "DROP FUNCTION IF EXISTS check_notification_rate_limit(UUID, VARCHAR, INTEGER, INTEGER) CASCADE;" || true

    # Remove migration records
    plugin_db_query "DELETE FROM plugin_migrations WHERE plugin_name = 'notifications';" || true

    # Clean up directories (optional - keep by default for safety)
    # Uncomment to remove cache/logs on uninstall
    # rm -rf "${HOME}/.nself/cache/plugins/notifications"
    # rm -rf "${HOME}/.nself/logs/plugins/notifications"
    # rm -rf "${HOME}/.nself/templates/notifications"

    plugin_success "Notifications plugin uninstalled successfully!"

    printf "\n"
    printf "Note: Cache and log files were preserved at:\n"
    printf "  ${HOME}/.nself/cache/plugins/notifications\n"
    printf "  ${HOME}/.nself/logs/plugins/notifications\n"
    printf "  ${HOME}/.nself/templates/notifications\n"
    printf "\n"
    printf "To remove them manually:\n"
    printf "  rm -rf ~/.nself/cache/plugins/notifications\n"
    printf "  rm -rf ~/.nself/logs/plugins/notifications\n"
    printf "  rm -rf ~/.nself/templates/notifications\n"
    printf "\n"
}

# Run uninstallation
uninstall_notifications_plugin
