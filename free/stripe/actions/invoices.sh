#!/bin/bash
# =============================================================================
# Stripe Invoices Action
# View synced invoice data
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Commands
# =============================================================================

list_invoices() {
    local status="${1:-all}"
    local limit="${2:-20}"

    printf "\n=== Stripe Invoices (%s) ===\n\n" "$status"

    local status_filter=""
    if [[ "$status" != "all" ]]; then
        status_filter="AND i.status = '$status'"
    fi

    plugin_db_query "
        SELECT
            i.id,
            c.email AS customer,
            i.status,
            i.amount_due / 100.0 AS amount_due,
            i.amount_paid / 100.0 AS amount_paid,
            i.currency,
            CASE WHEN i.paid THEN 'Yes' ELSE 'No' END AS paid,
            to_char(i.created_at, 'YYYY-MM-DD') AS created
        FROM stripe_invoices i
        JOIN stripe_customers c ON i.customer_id = c.id
        WHERE 1=1 $status_filter
        ORDER BY i.created_at DESC
        LIMIT $limit;
    "
}

get_invoice() {
    local invoice_id="$1"

    printf "\n=== Invoice: %s ===\n\n" "$invoice_id"

    plugin_db_query "
        SELECT
            i.id,
            i.number,
            i.status,
            c.id AS customer_id,
            c.email AS customer_email,
            c.name AS customer_name,
            i.amount_due / 100.0 AS amount_due,
            i.amount_paid / 100.0 AS amount_paid,
            i.amount_remaining / 100.0 AS amount_remaining,
            i.currency,
            i.paid,
            i.billing_reason,
            i.subscription_id,
            i.period_start,
            i.period_end,
            i.due_date,
            i.hosted_invoice_url,
            i.created_at,
            i.synced_at
        FROM stripe_invoices i
        JOIN stripe_customers c ON i.customer_id = c.id
        WHERE i.id = '$invoice_id';
    "
}

invoice_stats() {
    printf "\n=== Invoice Statistics ===\n\n"

    printf "By status:\n"
    plugin_db_query "
        SELECT
            status,
            COUNT(*) AS count,
            SUM(amount_due) / 100.0 AS total_due,
            SUM(amount_paid) / 100.0 AS total_paid
        FROM stripe_invoices
        GROUP BY status
        ORDER BY count DESC;
    "

    printf "\nLast 30 days:\n"
    plugin_db_query "
        SELECT
            COUNT(*) AS invoices,
            SUM(amount_paid) / 100.0 AS revenue,
            SUM(CASE WHEN paid THEN 1 ELSE 0 END)::float / NULLIF(COUNT(*), 0) * 100 AS paid_pct
        FROM stripe_invoices
        WHERE created_at > NOW() - INTERVAL '30 days';
    "

    printf "\nOverdue (open & past due date):\n"
    plugin_db_query "
        SELECT
            COUNT(*) AS count,
            SUM(amount_remaining) / 100.0 AS total_outstanding
        FROM stripe_invoices
        WHERE status = 'open'
          AND due_date < NOW();
    "
}

recent_payments() {
    local limit="${1:-10}"

    printf "\n=== Recent Paid Invoices ===\n\n"

    plugin_db_query "
        SELECT
            i.id,
            c.email AS customer,
            i.amount_paid / 100.0 AS amount,
            i.currency,
            to_char(i.created_at, 'YYYY-MM-DD HH24:MI') AS paid_at
        FROM stripe_invoices i
        JOIN stripe_customers c ON i.customer_id = c.id
        WHERE i.paid = TRUE
        ORDER BY i.created_at DESC
        LIMIT $limit;
    "
}

failed_payments() {
    local limit="${1:-10}"

    printf "\n=== Failed/Uncollectible Invoices ===\n\n"

    plugin_db_query "
        SELECT
            i.id,
            c.email AS customer,
            i.amount_due / 100.0 AS amount,
            i.currency,
            i.status,
            i.attempt_count AS attempts,
            to_char(i.created_at, 'YYYY-MM-DD') AS created
        FROM stripe_invoices i
        JOIN stripe_customers c ON i.customer_id = c.id
        WHERE i.status IN ('open', 'uncollectible')
          AND i.paid = FALSE
        ORDER BY i.amount_due DESC
        LIMIT $limit;
    "
}

# =============================================================================
# Main
# =============================================================================

show_help() {
    printf "Usage: nself plugin stripe invoices <command> [args]\n\n"
    printf "Commands:\n"
    printf "  list [status] [limit]  List invoices (default: all, 20)\n"
    printf "  get <id>               Get invoice details\n"
    printf "  stats                  Show invoice statistics\n"
    printf "  recent [limit]         Show recent paid invoices\n"
    printf "  failed [limit]         Show failed/outstanding invoices\n\n"
    printf "Status values: draft, open, paid, uncollectible, void, all\n\n"
    printf "Examples:\n"
    printf "  nself plugin stripe invoices list paid 50\n"
    printf "  nself plugin stripe invoices get in_abc123\n"
    printf "  nself plugin stripe invoices failed\n"
}

main() {
    local command="${1:-list}"
    shift || true

    case "$command" in
        list)
            list_invoices "${1:-all}" "${2:-20}"
            ;;
        get)
            if [[ -z "${1:-}" ]]; then
                plugin_error "Invoice ID required"
                return 1
            fi
            get_invoice "$1"
            ;;
        stats|statistics)
            invoice_stats
            ;;
        recent)
            recent_payments "${1:-10}"
            ;;
        failed|outstanding)
            failed_payments "${1:-10}"
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
