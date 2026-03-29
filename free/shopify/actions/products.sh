#!/usr/bin/env bash
# Shopify Plugin - Products Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin shopify products - Product management"
    echo ""
    echo "Usage: nself plugin shopify products [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List products"
    echo "  show <id>         Show product details"
    echo "  stats             Product statistics"
    echo "  low-stock         Show low stock items"
    echo ""
    echo "Options:"
    echo "  --vendor <name>   Filter by vendor"
    echo "  --status <status> Filter by status (active, archived, draft)"
    echo "  --limit <n>       Limit results (default: 50)"
    echo ""
}

list_products() {
    local vendor=""
    local status=""
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --vendor) vendor="$2"; shift 2 ;;
            --status) status="$2"; shift 2 ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ -n "$vendor" ]] && where_clause+=" AND vendor = '$vendor'"
    [[ -n "$status" ]] && where_clause+=" AND status = '$status'"

    printf "%-12s %-40s %-15s %-10s %-10s\n" "ID" "Title" "Vendor" "Status" "Variants"
    printf "%-12s %-40s %-15s %-10s %-10s\n" "------------" "----------------------------------------" "---------------" "----------" "----------"

    plugin_db_query "SELECT p.id, SUBSTRING(p.title, 1, 40), COALESCE(p.vendor, '-'), p.status,
                     (SELECT COUNT(*) FROM shopify_variants v WHERE v.product_id = p.id)
                     FROM shopify_products p
                     $where_clause
                     ORDER BY p.updated_at DESC
                     LIMIT $limit" | while IFS='|' read -r id title vendor status variants; do
        printf "%-12s %-40s %-15s %-10s %-10s\n" "$id" "$title" "$vendor" "$status" "$variants"
    done
}

show_product() {
    local product_id="$1"

    if [[ -z "$product_id" ]]; then
        plugin_log "error" "Product ID required"
        return 1
    fi

    echo "Product Details"
    echo "==============="
    plugin_db_query "SELECT
        'ID: ' || id,
        'Title: ' || title,
        'Vendor: ' || COALESCE(vendor, 'n/a'),
        'Type: ' || COALESCE(product_type, 'n/a'),
        'Status: ' || status,
        'Handle: ' || handle,
        'Tags: ' || COALESCE(tags, ''),
        'Created: ' || created_at,
        'Updated: ' || updated_at
    FROM shopify_products WHERE id = $product_id"

    echo ""
    echo "Variants:"
    plugin_db_query "SELECT
        '  ' || title || ' - $' || price || ' (SKU: ' || COALESCE(sku, 'n/a') || ', Stock: ' || inventory_quantity || ')'
    FROM shopify_variants WHERE product_id = $product_id ORDER BY position"
}

show_stats() {
    echo "Product Statistics"
    echo "=================="
    echo ""

    echo "By Status:"
    plugin_db_query "SELECT status, COUNT(*) FROM shopify_products GROUP BY status ORDER BY COUNT(*) DESC" | while IFS='|' read -r status count; do
        printf "  %-15s %s\n" "$status" "$count"
    done

    echo ""
    echo "By Vendor (Top 10):"
    plugin_db_query "SELECT COALESCE(vendor, 'No vendor'), COUNT(*)
                     FROM shopify_products
                     GROUP BY vendor
                     ORDER BY COUNT(*) DESC
                     LIMIT 10" | while IFS='|' read -r vendor count; do
        printf "  %-25s %s\n" "$vendor" "$count"
    done

    echo ""
    echo "Inventory Summary:"
    plugin_db_query "SELECT
        'Total Products: ' || COUNT(DISTINCT product_id),
        'Total Variants: ' || COUNT(*),
        'Total Stock: ' || SUM(inventory_quantity),
        'Low Stock (<5): ' || COUNT(*) FILTER (WHERE inventory_quantity < 5),
        'Out of Stock: ' || COUNT(*) FILTER (WHERE inventory_quantity = 0)
    FROM shopify_variants"
}

show_low_stock() {
    echo "Low Stock Items (< 5 units)"
    echo "==========================="
    echo ""

    printf "%-40s %-20s %-10s %-10s\n" "Product" "Variant" "SKU" "Stock"
    printf "%-40s %-20s %-10s %-10s\n" "----------------------------------------" "--------------------" "----------" "----------"

    plugin_db_query "SELECT SUBSTRING(p.title, 1, 40), SUBSTRING(v.title, 1, 20), COALESCE(v.sku, '-'), v.inventory_quantity
                     FROM shopify_variants v
                     JOIN shopify_products p ON v.product_id = p.id
                     WHERE v.inventory_quantity < 5
                       AND p.status = 'active'
                     ORDER BY v.inventory_quantity ASC
                     LIMIT 50" | while IFS='|' read -r product variant sku stock; do
        printf "%-40s %-20s %-10s %-10s\n" "$product" "$variant" "$sku" "$stock"
    done
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_products "$@" ;;
        show) show_product "$@" ;;
        stats) show_stats ;;
        low-stock) show_low_stock ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
