#!/usr/bin/env bash
# Shopify Plugin - Orders Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin shopify orders - Order management"
    echo ""
    echo "Usage: nself plugin shopify orders [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List orders"
    echo "  show <id>         Show order details"
    echo "  pending           List pending orders"
    echo "  unfulfilled       List unfulfilled orders"
    echo "  stats             Order statistics"
    echo ""
    echo "Options:"
    echo "  --status <status> Filter by financial status"
    echo "  --limit <n>       Limit results (default: 50)"
    echo ""
}

list_orders() {
    local status=""
    local fulfillment=""
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --status) status="$2"; shift 2 ;;
            --fulfillment) fulfillment="$2"; shift 2 ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE test = false"
    [[ -n "$status" ]] && where_clause+=" AND financial_status = '$status'"
    [[ -n "$fulfillment" ]] && where_clause+=" AND fulfillment_status = '$fulfillment'"

    printf "%-12s %-12s %-12s %-15s %-12s %-20s\n" "Order" "Name" "Total" "Payment" "Fulfillment" "Created"
    printf "%-12s %-12s %-12s %-15s %-12s %-20s\n" "------------" "------------" "------------" "---------------" "------------" "--------------------"

    plugin_db_query "SELECT id, name, total_price, financial_status, COALESCE(fulfillment_status, 'unfulfilled'), created_at
                     FROM shopify_orders
                     $where_clause
                     ORDER BY created_at DESC
                     LIMIT $limit" | while IFS='|' read -r id name total payment fulfillment created; do
        printf "%-12s %-12s $%-11s %-15s %-12s %-20s\n" "$id" "$name" "$total" "$payment" "$fulfillment" "${created:0:10}"
    done
}

show_order() {
    local order_id="$1"

    if [[ -z "$order_id" ]]; then
        plugin_log "error" "Order ID required"
        return 1
    fi

    echo "Order Details"
    echo "============="
    plugin_db_query "SELECT
        'Order: ' || name,
        'ID: ' || id,
        'Email: ' || COALESCE(email, 'n/a'),
        'Financial Status: ' || financial_status,
        'Fulfillment Status: ' || COALESCE(fulfillment_status, 'unfulfilled'),
        'Currency: ' || currency,
        'Subtotal: $' || subtotal_price,
        'Discounts: $' || COALESCE(total_discounts, '0'),
        'Tax: $' || COALESCE(total_tax, '0'),
        'Total: $' || total_price,
        'Gateway: ' || COALESCE(gateway, 'n/a'),
        'Created: ' || created_at
    FROM shopify_orders WHERE id = $order_id OR order_number = $order_id"

    echo ""
    echo "Line Items:"
    plugin_db_query "SELECT
        '  ' || quantity || 'x ' || title || ' @ $' || price || ' = $' || (quantity * price::numeric)
    FROM shopify_order_items WHERE order_id = $order_id"

    echo ""
    echo "Shipping Address:"
    plugin_db_query "SELECT shipping_address::text FROM shopify_orders WHERE id = $order_id" | jq -r '
        if . then
            "  " + (.first_name // "") + " " + (.last_name // "") + "\n" +
            "  " + (.address1 // "") + "\n" +
            (if .address2 then "  " + .address2 + "\n" else "" end) +
            "  " + (.city // "") + ", " + (.province_code // "") + " " + (.zip // "") + "\n" +
            "  " + (.country // "")
        else
            "  No shipping address"
        end
    ' 2>/dev/null || echo "  No shipping address"
}

show_pending() {
    list_orders --status pending "$@"
}

show_unfulfilled() {
    list_orders --fulfillment unfulfilled "$@"
}

show_stats() {
    echo "Order Statistics"
    echo "================"
    echo ""

    echo "Summary (excluding test orders):"
    plugin_db_query "SELECT
        'Total Orders: ' || COUNT(*),
        'Total Revenue: $' || COALESCE(SUM(total_price), 0),
        'Average Order: $' || COALESCE(ROUND(AVG(total_price), 2), 0)
    FROM shopify_orders WHERE test = false AND financial_status = 'paid'"

    echo ""
    echo "By Financial Status:"
    plugin_db_query "SELECT financial_status, COUNT(*), SUM(total_price)
                     FROM shopify_orders
                     WHERE test = false
                     GROUP BY financial_status
                     ORDER BY COUNT(*) DESC" | while IFS='|' read -r status count total; do
        printf "  %-15s %5s orders ($%s)\n" "$status" "$count" "$total"
    done

    echo ""
    echo "By Fulfillment Status:"
    plugin_db_query "SELECT COALESCE(fulfillment_status, 'unfulfilled'), COUNT(*)
                     FROM shopify_orders
                     WHERE test = false
                     GROUP BY fulfillment_status" | while IFS='|' read -r status count; do
        printf "  %-15s %s\n" "$status" "$count"
    done

    echo ""
    echo "Recent Daily Sales (Last 7 Days):"
    plugin_db_query "SELECT DATE(created_at), COUNT(*), SUM(total_price)
                     FROM shopify_orders
                     WHERE test = false AND financial_status = 'paid'
                       AND created_at > NOW() - INTERVAL '7 days'
                     GROUP BY DATE(created_at)
                     ORDER BY DATE(created_at) DESC" | while IFS='|' read -r date count total; do
        printf "  %-12s %3s orders ($%s)\n" "$date" "$count" "$total"
    done
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_orders "$@" ;;
        show) show_order "$@" ;;
        pending) show_pending "$@" ;;
        unfulfilled) show_unfulfilled "$@" ;;
        stats) show_stats ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
