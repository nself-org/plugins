#!/usr/bin/env bash
# Shopify Plugin - Customers Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin shopify customers - Customer management"
    echo ""
    echo "Usage: nself plugin shopify customers [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List customers"
    echo "  show <id>         Show customer details"
    echo "  top               Top customers by spending"
    echo "  stats             Customer statistics"
    echo ""
}

list_customers() {
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    printf "%-12s %-30s %-10s %-12s %-10s\n" "ID" "Email" "Orders" "Spent" "State"
    printf "%-12s %-30s %-10s %-12s %-10s\n" "------------" "------------------------------" "----------" "------------" "----------"

    plugin_db_query "SELECT id, COALESCE(email, '-'), orders_count, total_spent, state
                     FROM shopify_customers
                     ORDER BY total_spent DESC
                     LIMIT $limit" | while IFS='|' read -r id email orders spent state; do
        printf "%-12s %-30s %-10s $%-11s %-10s\n" "$id" "${email:0:30}" "$orders" "$spent" "$state"
    done
}

show_customer() {
    local customer_id="$1"

    if [[ -z "$customer_id" ]]; then
        plugin_log "error" "Customer ID required"
        return 1
    fi

    echo "Customer Details"
    echo "================"
    plugin_db_query "SELECT
        'ID: ' || id,
        'Name: ' || COALESCE(first_name, '') || ' ' || COALESCE(last_name, ''),
        'Email: ' || COALESCE(email, 'n/a'),
        'Phone: ' || COALESCE(phone, 'n/a'),
        'State: ' || state,
        'Orders: ' || orders_count,
        'Total Spent: $' || total_spent,
        'Avg Order: $' || CASE WHEN orders_count > 0 THEN ROUND(total_spent / orders_count, 2) ELSE 0 END,
        'Accepts Marketing: ' || accepts_marketing,
        'Verified Email: ' || verified_email,
        'Tags: ' || COALESCE(tags, ''),
        'Customer Since: ' || created_at
    FROM shopify_customers WHERE id = $customer_id"

    echo ""
    echo "Recent Orders:"
    plugin_db_query "SELECT name, total_price, financial_status, created_at
                     FROM shopify_orders
                     WHERE customer_id = $customer_id
                     ORDER BY created_at DESC
                     LIMIT 5" | while IFS='|' read -r name total status created; do
        printf "  %s - \$%s (%s) - %s\n" "$name" "$total" "$status" "${created:0:10}"
    done
}

show_top() {
    echo "Top Customers by Spending"
    echo "========================="
    echo ""

    printf "%-5s %-30s %-10s %-12s %-12s\n" "Rank" "Email" "Orders" "Total Spent" "Avg Order"
    printf "%-5s %-30s %-10s %-12s %-12s\n" "-----" "------------------------------" "----------" "------------" "------------"

    local rank=1
    plugin_db_query "SELECT email, orders_count, total_spent,
                     CASE WHEN orders_count > 0 THEN ROUND(total_spent / orders_count, 2) ELSE 0 END
                     FROM shopify_customers
                     WHERE orders_count > 0
                     ORDER BY total_spent DESC
                     LIMIT 20" | while IFS='|' read -r email orders spent avg; do
        printf "%-5s %-30s %-10s $%-11s $%-11s\n" "$rank" "${email:0:30}" "$orders" "$spent" "$avg"
        ((rank++))
    done
}

show_stats() {
    echo "Customer Statistics"
    echo "==================="
    echo ""

    echo "Summary:"
    plugin_db_query "SELECT
        'Total Customers: ' || COUNT(*),
        'With Orders: ' || COUNT(*) FILTER (WHERE orders_count > 0),
        'Total Revenue: $' || COALESCE(SUM(total_spent), 0),
        'Avg Customer Value: $' || COALESCE(ROUND(AVG(total_spent) FILTER (WHERE orders_count > 0), 2), 0)
    FROM shopify_customers"

    echo ""
    echo "By State:"
    plugin_db_query "SELECT state, COUNT(*) FROM shopify_customers GROUP BY state ORDER BY COUNT(*) DESC" | while IFS='|' read -r state count; do
        printf "  %-15s %s\n" "$state" "$count"
    done

    echo ""
    echo "Marketing Opt-in:"
    plugin_db_query "SELECT accepts_marketing, COUNT(*)
                     FROM shopify_customers
                     GROUP BY accepts_marketing" | while IFS='|' read -r accepts count; do
        local label="No"
        [[ "$accepts" == "t" ]] && label="Yes"
        printf "  %-15s %s\n" "$label" "$count"
    done

    echo ""
    echo "Customer Segments:"
    plugin_db_query "SELECT
        CASE
            WHEN total_spent >= 1000 THEN 'VIP ($1000+)'
            WHEN total_spent >= 500 THEN 'High ($500-999)'
            WHEN total_spent >= 100 THEN 'Medium ($100-499)'
            WHEN total_spent > 0 THEN 'Low ($1-99)'
            ELSE 'No purchases'
        END AS segment,
        COUNT(*)
    FROM shopify_customers
    GROUP BY segment
    ORDER BY MIN(total_spent) DESC" | while IFS='|' read -r segment count; do
        printf "  %-20s %s\n" "$segment" "$count"
    done
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_customers "$@" ;;
        show) show_customer "$@" ;;
        top) show_top ;;
        stats) show_stats ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
