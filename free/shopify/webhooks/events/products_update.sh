#!/usr/bin/env bash
# Shopify Webhook Handler - products/update

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_product_update() {
    local event_id="$1"
    local payload="$2"

    plugin_log "info" "Handling products/update event"

    local product_id title
    product_id=$(echo "$payload" | jq -r '.id')
    title=$(echo "$payload" | jq -r '.title')

    plugin_log "debug" "Product updated: $title ($product_id)"

    # Update product
    plugin_db_query "UPDATE shopify_products SET
        title = $(echo "$payload" | jq '.title'),
        body_html = $(echo "$payload" | jq '.body_html'),
        vendor = $(echo "$payload" | jq '.vendor'),
        product_type = $(echo "$payload" | jq '.product_type'),
        status = $(echo "$payload" | jq '.status'),
        tags = $(echo "$payload" | jq '.tags'),
        images = $(echo "$payload" | jq '.images'),
        options = $(echo "$payload" | jq '.options'),
        updated_at = $(echo "$payload" | jq '.updated_at'),
        synced_at = NOW()
    WHERE id = $product_id"

    # Update variants
    echo "$payload" | jq -c '.variants[]' 2>/dev/null | while read -r variant; do
        plugin_db_query "INSERT INTO shopify_variants (
            id, product_id, title, price, compare_at_price, sku, barcode,
            position, grams, weight, weight_unit, inventory_item_id,
            inventory_quantity, inventory_policy, requires_shipping, taxable,
            option1, option2, option3, created_at, updated_at
        ) VALUES (
            $(echo "$variant" | jq '.id'),
            $product_id,
            $(echo "$variant" | jq '.title'),
            $(echo "$variant" | jq '.price'),
            $(echo "$variant" | jq '.compare_at_price'),
            $(echo "$variant" | jq '.sku'),
            $(echo "$variant" | jq '.barcode'),
            $(echo "$variant" | jq '.position'),
            $(echo "$variant" | jq '.grams'),
            $(echo "$variant" | jq '.weight'),
            $(echo "$variant" | jq '.weight_unit'),
            $(echo "$variant" | jq '.inventory_item_id'),
            $(echo "$variant" | jq '.inventory_quantity'),
            $(echo "$variant" | jq '.inventory_policy'),
            $(echo "$variant" | jq '.requires_shipping'),
            $(echo "$variant" | jq '.taxable'),
            $(echo "$variant" | jq '.option1'),
            $(echo "$variant" | jq '.option2'),
            $(echo "$variant" | jq '.option3'),
            $(echo "$variant" | jq '.created_at'),
            $(echo "$variant" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            title = EXCLUDED.title,
            price = EXCLUDED.price,
            compare_at_price = EXCLUDED.compare_at_price,
            sku = EXCLUDED.sku,
            inventory_quantity = EXCLUDED.inventory_quantity,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"
    done

    plugin_log "success" "Product updated: $title"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_product_update "$@"
