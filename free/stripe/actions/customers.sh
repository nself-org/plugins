#!/bin/bash
# =============================================================================
# Stripe Customers Action
# View and manage synced customer data
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Commands
# =============================================================================

list_customers() {
    local limit="${1:-20}"
    local offset="${2:-0}"

    printf "\n=== Stripe Customers ===\n\n"

    plugin_db_query "
        SELECT
            id,
            COALESCE(email, '-') AS email,
            COALESCE(name, '-') AS name,
            COALESCE(currency, '-') AS currency,
            balance,
            CASE WHEN delinquent THEN 'Yes' ELSE 'No' END AS delinquent,
            to_char(created_at, 'YYYY-MM-DD') AS created
        FROM stripe_customers
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT $limit OFFSET $offset;
    "
}

get_customer() {
    local customer_id="$1"

    printf "\n=== Customer: %s ===\n\n" "$customer_id"

    # Customer details
    plugin_db_query "
        SELECT
            id,
            email,
            name,
            phone,
            description,
            currency,
            balance,
            delinquent,
            created_at,
            synced_at
        FROM stripe_customers
        WHERE id = '$customer_id';
    "

    # Subscriptions
    printf "\n--- Subscriptions ---\n"
    plugin_db_query "
        SELECT
            id,
            status,
            current_period_start,
            current_period_end,
            cancel_at_period_end
        FROM stripe_subscriptions
        WHERE customer_id = '$customer_id'
        ORDER BY created_at DESC;
    "

    # Recent invoices
    printf "\n--- Recent Invoices ---\n"
    plugin_db_query "
        SELECT
            id,
            status,
            amount_due,
            amount_paid,
            currency,
            created_at
        FROM stripe_invoices
        WHERE customer_id = '$customer_id'
        ORDER BY created_at DESC
        LIMIT 5;
    "
}

search_customers() {
    local query="$1"

    printf "\n=== Search: %s ===\n\n" "$query"

    plugin_db_query "
        SELECT
            id,
            email,
            name,
            currency,
            created_at
        FROM stripe_customers
        WHERE deleted_at IS NULL
          AND (
            email ILIKE '%${query}%'
            OR name ILIKE '%${query}%'
            OR id ILIKE '%${query}%'
          )
        ORDER BY created_at DESC
        LIMIT 20;
    "
}

count_customers() {
    printf "\n=== Customer Statistics ===\n\n"

    printf "Total customers: "
    plugin_db_query "SELECT COUNT(*) FROM stripe_customers WHERE deleted_at IS NULL;"

    printf "With subscriptions: "
    plugin_db_query "SELECT COUNT(DISTINCT customer_id) FROM stripe_subscriptions WHERE status IN ('active', 'trialing');"

    printf "Delinquent: "
    plugin_db_query "SELECT COUNT(*) FROM stripe_customers WHERE delinquent = TRUE AND deleted_at IS NULL;"

    printf "\nBy currency:\n"
    plugin_db_query "
        SELECT
            COALESCE(currency, 'unknown') AS currency,
            COUNT(*) AS count
        FROM stripe_customers
        WHERE deleted_at IS NULL
        GROUP BY currency
        ORDER BY count DESC;
    "
}

# =============================================================================
# Main
# =============================================================================

show_help() {
    printf "Usage: nself plugin stripe customers <command> [args]\n\n"
    printf "Commands:\n"
    printf "  list [limit] [offset]  List customers (default: 20)\n"
    printf "  get <id>               Get customer details\n"
    printf "  search <query>         Search customers by email, name, or ID\n"
    printf "  count                  Show customer statistics\n\n"
    printf "Examples:\n"
    printf "  nself plugin stripe customers list\n"
    printf "  nself plugin stripe customers get cus_abc123\n"
    printf "  nself plugin stripe customers search john@example.com\n"
}

main() {
    local command="${1:-list}"
    shift || true

    case "$command" in
        list)
            list_customers "${1:-20}" "${2:-0}"
            ;;
        get)
            if [[ -z "${1:-}" ]]; then
                plugin_error "Customer ID required"
                return 1
            fi
            get_customer "$1"
            ;;
        search)
            if [[ -z "${1:-}" ]]; then
                plugin_error "Search query required"
                return 1
            fi
            search_customers "$1"
            ;;
        count|stats)
            count_customers
            ;;
        -h|--help|help)
            show_help
            ;;
        *)
            plugin_error "Unknown command: $command"
            show_help
            return 1
            ;;
    esac
}

main "$@"
