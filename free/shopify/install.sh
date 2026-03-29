#!/usr/bin/env bash
# Shopify Plugin - Installation Script

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="shopify"

# Source shared utilities
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Pre-flight Checks
# ============================================================================

preflight_checks() {
    plugin_log "info" "Running pre-flight checks..."

    # Check required environment variables
    if [[ -z "${SHOPIFY_STORE:-}" ]]; then
        plugin_log "error" "SHOPIFY_STORE is required"
        plugin_log "info" "Format: your-store.myshopify.com or just 'your-store'"
        return 1
    fi

    if [[ -z "${SHOPIFY_ACCESS_TOKEN:-}" ]]; then
        plugin_log "error" "SHOPIFY_ACCESS_TOKEN is required"
        plugin_log "info" "Create a custom app in your Shopify admin to get an access token"
        return 1
    fi

    # Normalize store domain
    local store_domain="$SHOPIFY_STORE"
    if [[ ! "$store_domain" == *".myshopify.com" ]]; then
        store_domain="${store_domain}.myshopify.com"
    fi

    # Test API access
    plugin_log "info" "Testing Shopify API access..."
    local api_version="${SHOPIFY_API_VERSION:-2024-01}"
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "X-Shopify-Access-Token: $SHOPIFY_ACCESS_TOKEN" \
        "https://${store_domain}/admin/api/${api_version}/shop.json")

    if [[ "$response" != "200" ]]; then
        plugin_log "error" "Shopify API returned status $response"
        plugin_log "info" "Check your SHOPIFY_STORE and SHOPIFY_ACCESS_TOKEN"
        return 1
    fi

    plugin_log "success" "Shopify API access verified"
    return 0
}

# ============================================================================
# Database Setup
# ============================================================================

setup_database() {
    plugin_log "info" "Setting up database schema..."

    local tables_exist
    tables_exist=$(plugin_db_query "SELECT COUNT(*) FROM information_schema.tables WHERE table_name LIKE 'shopify_%'" 2>/dev/null || echo "0")

    if [[ "$tables_exist" -gt 0 ]]; then
        plugin_log "info" "Shopify tables already exist, checking for updates..."
        return 0
    fi

    if [[ -f "${PLUGIN_DIR}/schema/tables.sql" ]]; then
        plugin_log "info" "Applying database schema..."
        plugin_db_exec_file "${PLUGIN_DIR}/schema/tables.sql"
        plugin_log "success" "Database schema applied"
    else
        plugin_log "error" "Schema file not found"
        return 1
    fi

    return 0
}

# ============================================================================
# Webhook Setup
# ============================================================================

setup_webhooks() {
    plugin_log "info" "Configuring webhook handlers..."

    local webhook_path="${SHOPIFY_WEBHOOK_PATH:-/webhooks/shopify}"
    plugin_log "info" "Webhook endpoint: $webhook_path"

    if [[ -n "${SHOPIFY_WEBHOOK_SECRET:-}" ]]; then
        plugin_log "success" "Webhook signature verification enabled"
    else
        plugin_log "warning" "SHOPIFY_WEBHOOK_SECRET not set"
    fi

    local webhook_runtime_dir="${HOME}/.nself/plugins/${PLUGIN_NAME}/webhooks"
    mkdir -p "$webhook_runtime_dir"

    if [[ -d "${PLUGIN_DIR}/webhooks" ]]; then
        cp -r "${PLUGIN_DIR}/webhooks/"* "$webhook_runtime_dir/" 2>/dev/null || true
        chmod +x "$webhook_runtime_dir"/*.sh 2>/dev/null || true
        chmod +x "$webhook_runtime_dir/events/"*.sh 2>/dev/null || true
    fi

    return 0
}

# ============================================================================
# Initial Sync
# ============================================================================

initial_sync() {
    plugin_log "info" "Running initial data sync..."

    if [[ -x "${PLUGIN_DIR}/actions/sync.sh" ]]; then
        bash "${PLUGIN_DIR}/actions/sync.sh" --initial
    else
        plugin_log "warning" "Sync action not found, skipping initial sync"
    fi

    return 0
}

# ============================================================================
# Main Installation
# ============================================================================

main() {
    plugin_log "info" "Installing Shopify plugin..."

    if ! preflight_checks; then
        plugin_log "error" "Pre-flight checks failed"
        return 1
    fi

    if ! setup_database; then
        plugin_log "error" "Database setup failed"
        return 1
    fi

    if ! setup_webhooks; then
        plugin_log "error" "Webhook setup failed"
        return 1
    fi

    if [[ "${SKIP_INITIAL_SYNC:-}" != "true" ]]; then
        initial_sync || plugin_log "warning" "Initial sync had issues, continuing..."
    fi

    plugin_mark_installed "$PLUGIN_NAME"

    plugin_log "success" "Shopify plugin installed successfully"

    echo ""
    echo "Usage:"
    echo "  nself plugin shopify sync        - Sync store data"
    echo "  nself plugin shopify products    - List products"
    echo "  nself plugin shopify orders      - View orders"
    echo "  nself plugin shopify customers   - View customers"
    echo "  nself plugin shopify inventory   - Check inventory"
    echo ""

    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
