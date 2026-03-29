#!/bin/bash
# =============================================================================
# Stripe Subscriptions Action
# View and manage synced subscription data
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Commands
# =============================================================================

list_subscriptions() {
    local status="${1:-active}"
    local limit="${2:-20}"

    printf "\n=== Stripe Subscriptions (%s) ===\n\n" "$status"

    local status_filter=""
    if [[ "$status" != "all" ]]; then
        status_filter="AND s.status = '$status'"
    fi

    plugin_db_query "
        SELECT
            s.id,
            c.email AS customer,
            s.status,
            to_char(s.current_period_start, 'YYYY-MM-DD') AS period_start,
            to_char(s.current_period_end, 'YYYY-MM-DD') AS period_end,
            CASE WHEN s.cancel_at_period_end THEN 'Yes' ELSE 'No' END AS canceling
        FROM stripe_subscriptions s
        JOIN stripe_customers c ON s.customer_id = c.id
        WHERE 1=1 $status_filter
        ORDER BY s.created_at DESC
        LIMIT $limit;
    "
}

get_subscription() {
    local sub_id="$1"

    printf "\n=== Subscription: %s ===\n\n" "$sub_id"

    plugin_db_query "
        SELECT
            s.id,
            s.status,
            c.id AS customer_id,
            c.email AS customer_email,
            c.name AS customer_name,
            s.current_period_start,
            s.current_period_end,
            s.cancel_at_period_end,
            s.canceled_at,
            s.ended_at,
            s.trial_start,
            s.trial_end,
            s.created_at,
            s.synced_at
        FROM stripe_subscriptions s
        JOIN stripe_customers c ON s.customer_id = c.id
        WHERE s.id = '$sub_id';
    "

    # Related invoices
    printf "\n--- Related Invoices ---\n"
    plugin_db_query "
        SELECT
            id,
            status,
            amount_due,
            amount_paid,
            currency,
            paid,
            created_at
        FROM stripe_invoices
        WHERE subscription_id = '$sub_id'
        ORDER BY created_at DESC
        LIMIT 10;
    "
}

subscription_stats() {
    printf "\n=== Subscription Statistics ===\n\n"

    printf "By status:\n"
    plugin_db_query "
        SELECT
            status,
            COUNT(*) AS count
        FROM stripe_subscriptions
        GROUP BY status
        ORDER BY count DESC;
    "

    printf "\nCanceling at period end:\n"
    plugin_db_query "
        SELECT COUNT(*) AS count
        FROM stripe_subscriptions
        WHERE cancel_at_period_end = TRUE
          AND status IN ('active', 'trialing');
    "

    printf "\nTrials ending this week:\n"
    plugin_db_query "
        SELECT COUNT(*) AS count
        FROM stripe_subscriptions
        WHERE status = 'trialing'
          AND trial_end BETWEEN NOW() AND NOW() + INTERVAL '7 days';
    "

    printf "\nExpiring this month:\n"
    plugin_db_query "
        SELECT COUNT(*) AS count
        FROM stripe_subscriptions
        WHERE status IN ('active', 'trialing')
          AND current_period_end BETWEEN NOW() AND NOW() + INTERVAL '30 days';
    "
}

mrr_report() {
    printf "\n=== Monthly Recurring Revenue ===\n\n"

    printf "Active MRR (estimated from subscription count):\n"
    plugin_db_query "
        SELECT
            COUNT(*) AS active_subscriptions,
            (SELECT COUNT(*) FROM stripe_customers WHERE deleted_at IS NULL) AS total_customers
        FROM stripe_subscriptions
        WHERE status IN ('active', 'trialing');
    "

    printf "\nNote: For accurate MRR, run 'nself plugin stripe sync' to get full pricing data.\n"
}

# =============================================================================
# Main
# =============================================================================

show_help() {
    printf "Usage: nself plugin stripe subscriptions <command> [args]\n\n"
    printf "Commands:\n"
    printf "  list [status] [limit]  List subscriptions (default: active, 20)\n"
    printf "  get <id>               Get subscription details\n"
    printf "  stats                  Show subscription statistics\n"
    printf "  mrr                    Show MRR report\n\n"
    printf "Status values: active, trialing, past_due, canceled, all\n\n"
    printf "Examples:\n"
    printf "  nself plugin stripe subscriptions list\n"
    printf "  nself plugin stripe subscriptions list all 50\n"
    printf "  nself plugin stripe subscriptions get sub_abc123\n"
}

main() {
    local command="${1:-list}"
    shift || true

    case "$command" in
        list)
            list_subscriptions "${1:-active}" "${2:-20}"
            ;;
        get)
            if [[ -z "${1:-}" ]]; then
                plugin_error "Subscription ID required"
                return 1
            fi
            get_subscription "$1"
            ;;
        stats|statistics)
            subscription_stats
            ;;
        mrr|revenue)
            mrr_report
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
