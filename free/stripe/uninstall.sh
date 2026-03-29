#!/bin/bash
# =============================================================================
# Stripe Plugin Uninstaller
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Uninstallation
# =============================================================================

uninstall_stripe_plugin() {
    local keep_data="${1:-false}"

    plugin_info "Uninstalling Stripe plugin..."

    if [[ "$keep_data" != "true" ]]; then
        printf "\n"
        printf "\033[33mWarning: This will delete all Stripe data from the database.\033[0m\n"
        printf "Tables to be removed:\n"
        printf "  - stripe_customers\n"
        printf "  - stripe_products\n"
        printf "  - stripe_prices\n"
        printf "  - stripe_subscriptions\n"
        printf "  - stripe_invoices\n"
        printf "  - stripe_payment_intents\n"
        printf "  - stripe_payment_methods\n"
        printf "  - stripe_webhook_events\n"
        printf "\n"

        read -r -p "Are you sure you want to continue? [y/N] " response
        response=$(echo "$response" | tr '[:upper:]' '[:lower:]')

        if [[ "$response" != "y" && "$response" != "yes" ]]; then
            plugin_info "Uninstallation cancelled"
            return 0
        fi

        # Drop tables
        plugin_info "Removing database tables..."

        plugin_db_query "DROP TABLE IF EXISTS stripe_webhook_events CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_payment_methods CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_payment_intents CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_invoices CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_subscriptions CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_prices CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_products CASCADE;" >/dev/null 2>&1 || true
        plugin_db_query "DROP TABLE IF EXISTS stripe_customers CASCADE;" >/dev/null 2>&1 || true

        # Remove migration records
        plugin_db_query "DELETE FROM _nself_plugin_migrations WHERE plugin_name = 'stripe';" >/dev/null 2>&1 || true
    else
        plugin_info "Keeping data in database (--keep-data flag set)"
    fi

    # Clean up cache and logs
    rm -rf "${HOME}/.nself/cache/plugins/stripe"
    rm -rf "${HOME}/.nself/logs/plugins/stripe"
    rm -rf "${HOME}/.nself/queue/webhooks/stripe"

    plugin_success "Stripe plugin uninstalled successfully!"
}

# Parse arguments
KEEP_DATA="false"
while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep-data)
            KEEP_DATA="true"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

# Run uninstallation
uninstall_stripe_plugin "$KEEP_DATA"
