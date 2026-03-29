#!/usr/bin/env bash
# Shopify Webhook Handler - orders/create

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_order_create() {
    local event_id="$1"
    local payload="$2"

    plugin_log "info" "Handling orders/create event"

    local order_id order_name total
    order_id=$(echo "$payload" | jq -r '.id')
    order_name=$(echo "$payload" | jq -r '.name')
    total=$(echo "$payload" | jq -r '.total_price')

    plugin_log "info" "New order: $order_name (\$$total)"

    # Upsert order
    local customer_id
    customer_id=$(echo "$payload" | jq -r '.customer.id // "null"')

    plugin_db_query "INSERT INTO shopify_orders (
        id, order_number, name, email, phone, customer_id,
        financial_status, fulfillment_status, currency,
        subtotal_price, total_discounts, total_price, total_tax,
        discount_codes, note, tags, gateway, source_name,
        billing_address, shipping_address, shipping_lines,
        processed_at, created_at, updated_at
    ) VALUES (
        $(echo "$payload" | jq '.id'),
        $(echo "$payload" | jq '.order_number'),
        $(echo "$payload" | jq '.name'),
        $(echo "$payload" | jq '.email'),
        $(echo "$payload" | jq '.phone'),
        $(if [[ "$customer_id" != "null" ]]; then echo "$customer_id"; else echo "NULL"; fi),
        $(echo "$payload" | jq '.financial_status'),
        $(echo "$payload" | jq '.fulfillment_status'),
        $(echo "$payload" | jq '.currency'),
        $(echo "$payload" | jq '.subtotal_price'),
        $(echo "$payload" | jq '.total_discounts'),
        $(echo "$payload" | jq '.total_price'),
        $(echo "$payload" | jq '.total_tax'),
        $(echo "$payload" | jq '.discount_codes'),
        $(echo "$payload" | jq '.note'),
        $(echo "$payload" | jq '.tags'),
        $(echo "$payload" | jq '.gateway'),
        $(echo "$payload" | jq '.source_name'),
        $(echo "$payload" | jq '.billing_address'),
        $(echo "$payload" | jq '.shipping_address'),
        $(echo "$payload" | jq '.shipping_lines'),
        $(echo "$payload" | jq '.processed_at'),
        $(echo "$payload" | jq '.created_at'),
        $(echo "$payload" | jq '.updated_at')
    ) ON CONFLICT (id) DO UPDATE SET
        financial_status = EXCLUDED.financial_status,
        fulfillment_status = EXCLUDED.fulfillment_status,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()"

    # Insert line items
    echo "$payload" | jq -c '.line_items[]' 2>/dev/null | while read -r item; do
        plugin_db_query "INSERT INTO shopify_order_items (
            id, order_id, product_id, variant_id, title, variant_title,
            sku, vendor, quantity, price, total_discount,
            fulfillment_status, fulfillable_quantity
        ) VALUES (
            $(echo "$item" | jq '.id'),
            $order_id,
            $(echo "$item" | jq '.product_id'),
            $(echo "$item" | jq '.variant_id'),
            $(echo "$item" | jq '.title'),
            $(echo "$item" | jq '.variant_title'),
            $(echo "$item" | jq '.sku'),
            $(echo "$item" | jq '.vendor'),
            $(echo "$item" | jq '.quantity'),
            $(echo "$item" | jq '.price'),
            $(echo "$item" | jq '.total_discount'),
            $(echo "$item" | jq '.fulfillment_status'),
            $(echo "$item" | jq '.fulfillable_quantity')
        ) ON CONFLICT (id) DO NOTHING"
    done

    plugin_log "success" "Order created: $order_name"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_order_create "$@"
