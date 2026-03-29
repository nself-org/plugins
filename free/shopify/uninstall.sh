#!/usr/bin/env bash
# Shopify Plugin - Uninstall Script

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="shopify"

source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

remove_database() {
    local keep_data="${1:-false}"

    if [[ "$keep_data" == "true" ]]; then
        plugin_log "info" "Keeping database tables (--keep-data specified)"
        return 0
    fi

    plugin_log "warning" "Removing Shopify database tables..."

    # Drop views first
    plugin_db_query "DROP VIEW IF EXISTS shopify_customer_value CASCADE" 2>/dev/null || true
    plugin_db_query "DROP VIEW IF EXISTS shopify_low_inventory CASCADE" 2>/dev/null || true
    plugin_db_query "DROP VIEW IF EXISTS shopify_top_products CASCADE" 2>/dev/null || true
    plugin_db_query "DROP VIEW IF EXISTS shopify_sales_overview CASCADE" 2>/dev/null || true

    # Drop tables in reverse dependency order
    plugin_db_query "DROP TABLE IF EXISTS shopify_webhook_events CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_inventory CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_order_items CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_orders CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_customers CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_collections CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_variants CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_products CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS shopify_shops CASCADE" 2>/dev/null || true

    plugin_log "success" "Database tables removed"
    return 0
}

remove_webhooks() {
    plugin_log "info" "Removing webhook handlers..."
    local webhook_runtime_dir="${HOME}/.nself/plugins/${PLUGIN_NAME}/webhooks"
    [[ -d "$webhook_runtime_dir" ]] && rm -rf "$webhook_runtime_dir"
    plugin_log "success" "Webhook handlers removed"
    return 0
}

remove_cache() {
    plugin_log "info" "Clearing plugin cache..."
    local cache_dir="${HOME}/.nself/cache/plugins/${PLUGIN_NAME}"
    [[ -d "$cache_dir" ]] && rm -rf "$cache_dir"
    plugin_log "success" "Cache cleared"
    return 0
}

main() {
    local keep_data=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --keep-data) keep_data=true; shift ;;
            *) shift ;;
        esac
    done

    plugin_log "info" "Uninstalling Shopify plugin..."

    remove_database "$keep_data"
    remove_webhooks
    remove_cache
    plugin_mark_uninstalled "$PLUGIN_NAME"

    plugin_log "success" "Shopify plugin uninstalled"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
