#!/bin/bash
# =============================================================================
# ID.me Plugin Uninstaller
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Uninstallation
# =============================================================================

uninstall_idme_plugin() {
    plugin_info "Uninstalling ID.me plugin..."

    # Prompt for data deletion
    plugin_warn "This will remove all ID.me verification data from the database."
    printf "Are you sure you want to continue? (yes/no): "
    read -r confirm

    if [[ "$confirm" != "yes" ]]; then
        plugin_info "Uninstall cancelled."
        return 0
    fi

    # Drop tables in reverse dependency order
    plugin_info "Removing database tables..."

    plugin_db_query "DROP VIEW IF EXISTS idme_recent_verifications CASCADE;" || true
    plugin_db_query "DROP VIEW IF EXISTS idme_group_summary CASCADE;" || true
    plugin_db_query "DROP VIEW IF EXISTS idme_verified_users CASCADE;" || true

    plugin_db_query "DROP TABLE IF EXISTS idme_webhook_events CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS idme_attributes CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS idme_badges CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS idme_groups CASCADE;" || true
    plugin_db_query "DROP TABLE IF EXISTS idme_verifications CASCADE;" || true

    # Drop triggers and functions
    plugin_db_query "DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;" || true

    # Remove migration records
    plugin_db_query "DELETE FROM plugin_migrations WHERE plugin_name = 'idme';" || true

    # Remove cache and logs (optional)
    printf "Remove cached data and logs? (yes/no): "
    read -r remove_cache
    if [[ "$remove_cache" == "yes" ]]; then
        rm -rf "${HOME}/.nself/cache/plugins/idme"
        rm -rf "${HOME}/.nself/logs/plugins/idme"
        plugin_info "Cache and logs removed"
    fi

    plugin_success "ID.me plugin uninstalled successfully!"
}

# Run uninstallation
uninstall_idme_plugin
