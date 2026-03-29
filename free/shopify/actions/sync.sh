#!/usr/bin/env bash
# Shopify Plugin - Sync Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# Configuration
API_VERSION="${SHOPIFY_API_VERSION:-2024-01}"
RATE_LIMIT_DELAY="${SHOPIFY_RATE_LIMIT_DELAY:-0.5}"

get_store_domain() {
    local store="$SHOPIFY_STORE"
    [[ ! "$store" == *".myshopify.com" ]] && store="${store}.myshopify.com"
    echo "$store"
}

shopify_api() {
    local endpoint="$1"
    local store_domain
    store_domain=$(get_store_domain)

    curl -s \
        -H "X-Shopify-Access-Token: $SHOPIFY_ACCESS_TOKEN" \
        -H "Content-Type: application/json" \
        "https://${store_domain}/admin/api/${API_VERSION}/${endpoint}"

    sleep "$RATE_LIMIT_DELAY"
}

shopify_api_paginated() {
    local endpoint="$1"
    local results="[]"
    local page_info=""
    local url="$endpoint"

    while true; do
        local response
        response=$(shopify_api "$url")

        local data_key
        data_key=$(echo "$endpoint" | sed 's/\.json.*//' | sed 's/.*\///')
        local items
        items=$(echo "$response" | jq ".${data_key} // []")

        results=$(echo "$results $items" | jq -s 'add')

        # Check for pagination link
        local next_link
        next_link=$(echo "$response" | jq -r '.next_page_info // empty')
        if [[ -z "$next_link" ]]; then
            break
        fi
        url="${endpoint}?page_info=${next_link}"
    done

    echo "$results"
}

sync_shop() {
    plugin_log "info" "Syncing shop info..."

    local shop
    shop=$(shopify_api "shop.json")
    local shop_data
    shop_data=$(echo "$shop" | jq '.shop')

    if [[ -z "$shop_data" ]] || [[ "$shop_data" == "null" ]]; then
        plugin_log "error" "Failed to fetch shop info"
        return 1
    fi

    local shop_id
    shop_id=$(echo "$shop_data" | jq -r '.id')

    plugin_db_query "INSERT INTO shopify_shops (
        id, name, email, domain, myshopify_domain, shop_owner,
        phone, address1, city, province, country, zip,
        currency, timezone, iana_timezone, plan_name, weight_unit,
        created_at, updated_at
    ) VALUES (
        $(echo "$shop_data" | jq '.id'),
        $(echo "$shop_data" | jq '.name'),
        $(echo "$shop_data" | jq '.email'),
        $(echo "$shop_data" | jq '.domain'),
        $(echo "$shop_data" | jq '.myshopify_domain'),
        $(echo "$shop_data" | jq '.shop_owner'),
        $(echo "$shop_data" | jq '.phone'),
        $(echo "$shop_data" | jq '.address1'),
        $(echo "$shop_data" | jq '.city'),
        $(echo "$shop_data" | jq '.province'),
        $(echo "$shop_data" | jq '.country'),
        $(echo "$shop_data" | jq '.zip'),
        $(echo "$shop_data" | jq '.currency'),
        $(echo "$shop_data" | jq '.timezone'),
        $(echo "$shop_data" | jq '.iana_timezone'),
        $(echo "$shop_data" | jq '.plan_name'),
        $(echo "$shop_data" | jq '.weight_unit'),
        $(echo "$shop_data" | jq '.created_at'),
        $(echo "$shop_data" | jq '.updated_at')
    ) ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        email = EXCLUDED.email,
        plan_name = EXCLUDED.plan_name,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()"

    echo "$shop_id"
}

sync_products() {
    local shop_id="$1"
    plugin_log "info" "Syncing products..."

    local products
    products=$(shopify_api_paginated "products.json?limit=250")

    local count
    count=$(echo "$products" | jq 'length')
    plugin_log "info" "Found $count products"

    echo "$products" | jq -c '.[]' | while read -r product; do
        local product_id
        product_id=$(echo "$product" | jq -r '.id')

        plugin_db_query "INSERT INTO shopify_products (
            id, shop_id, title, body_html, vendor, product_type, handle,
            status, tags, images, options, published_at, created_at, updated_at
        ) VALUES (
            $(echo "$product" | jq '.id'),
            $shop_id,
            $(echo "$product" | jq '.title'),
            $(echo "$product" | jq '.body_html'),
            $(echo "$product" | jq '.vendor'),
            $(echo "$product" | jq '.product_type'),
            $(echo "$product" | jq '.handle'),
            $(echo "$product" | jq '.status'),
            $(echo "$product" | jq '.tags'),
            $(echo "$product" | jq '.images'),
            $(echo "$product" | jq '.options'),
            $(echo "$product" | jq '.published_at'),
            $(echo "$product" | jq '.created_at'),
            $(echo "$product" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            title = EXCLUDED.title,
            body_html = EXCLUDED.body_html,
            vendor = EXCLUDED.vendor,
            status = EXCLUDED.status,
            tags = EXCLUDED.tags,
            images = EXCLUDED.images,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"

        # Sync variants
        echo "$product" | jq -c '.variants[]' 2>/dev/null | while read -r variant; do
            plugin_db_query "INSERT INTO shopify_variants (
                id, product_id, title, price, compare_at_price, sku, barcode,
                position, grams, weight, weight_unit, inventory_item_id,
                inventory_quantity, inventory_policy, inventory_management,
                requires_shipping, taxable, option1, option2, option3,
                created_at, updated_at
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
                $(echo "$variant" | jq '.inventory_management'),
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
                inventory_quantity = EXCLUDED.inventory_quantity,
                updated_at = EXCLUDED.updated_at,
                synced_at = NOW()"
        done
    done

    plugin_log "success" "Products synced"
}

sync_customers() {
    local shop_id="$1"
    plugin_log "info" "Syncing customers..."

    local customers
    customers=$(shopify_api_paginated "customers.json?limit=250")

    local count
    count=$(echo "$customers" | jq 'length')
    plugin_log "info" "Found $count customers"

    echo "$customers" | jq -c '.[]' | while read -r customer; do
        plugin_db_query "INSERT INTO shopify_customers (
            id, shop_id, email, first_name, last_name, phone,
            verified_email, accepts_marketing, orders_count, total_spent,
            state, note, tags, default_address, addresses,
            created_at, updated_at
        ) VALUES (
            $(echo "$customer" | jq '.id'),
            $shop_id,
            $(echo "$customer" | jq '.email'),
            $(echo "$customer" | jq '.first_name'),
            $(echo "$customer" | jq '.last_name'),
            $(echo "$customer" | jq '.phone'),
            $(echo "$customer" | jq '.verified_email'),
            $(echo "$customer" | jq '.accepts_marketing'),
            $(echo "$customer" | jq '.orders_count'),
            $(echo "$customer" | jq '.total_spent'),
            $(echo "$customer" | jq '.state'),
            $(echo "$customer" | jq '.note'),
            $(echo "$customer" | jq '.tags'),
            $(echo "$customer" | jq '.default_address'),
            $(echo "$customer" | jq '.addresses'),
            $(echo "$customer" | jq '.created_at'),
            $(echo "$customer" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            email = EXCLUDED.email,
            orders_count = EXCLUDED.orders_count,
            total_spent = EXCLUDED.total_spent,
            state = EXCLUDED.state,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"
    done

    plugin_log "success" "Customers synced"
}

sync_orders() {
    local shop_id="$1"
    plugin_log "info" "Syncing orders..."

    local orders
    orders=$(shopify_api_paginated "orders.json?status=any&limit=250")

    local count
    count=$(echo "$orders" | jq 'length')
    plugin_log "info" "Found $count orders"

    echo "$orders" | jq -c '.[]' | while read -r order; do
        local order_id
        order_id=$(echo "$order" | jq -r '.id')
        local customer_id
        customer_id=$(echo "$order" | jq -r '.customer.id // "null"')

        plugin_db_query "INSERT INTO shopify_orders (
            id, shop_id, order_number, name, email, phone,
            customer_id, financial_status, fulfillment_status,
            currency, subtotal_price, total_discounts, total_price,
            total_tax, total_weight, discount_codes, note, tags,
            gateway, source_name, billing_address, shipping_address,
            shipping_lines, processed_at, created_at, updated_at
        ) VALUES (
            $(echo "$order" | jq '.id'),
            $shop_id,
            $(echo "$order" | jq '.order_number'),
            $(echo "$order" | jq '.name'),
            $(echo "$order" | jq '.email'),
            $(echo "$order" | jq '.phone'),
            $(if [[ "$customer_id" != "null" ]]; then echo "$customer_id"; else echo "NULL"; fi),
            $(echo "$order" | jq '.financial_status'),
            $(echo "$order" | jq '.fulfillment_status'),
            $(echo "$order" | jq '.currency'),
            $(echo "$order" | jq '.subtotal_price'),
            $(echo "$order" | jq '.total_discounts'),
            $(echo "$order" | jq '.total_price'),
            $(echo "$order" | jq '.total_tax'),
            $(echo "$order" | jq '.total_weight'),
            $(echo "$order" | jq '.discount_codes'),
            $(echo "$order" | jq '.note'),
            $(echo "$order" | jq '.tags'),
            $(echo "$order" | jq '.gateway'),
            $(echo "$order" | jq '.source_name'),
            $(echo "$order" | jq '.billing_address'),
            $(echo "$order" | jq '.shipping_address'),
            $(echo "$order" | jq '.shipping_lines'),
            $(echo "$order" | jq '.processed_at'),
            $(echo "$order" | jq '.created_at'),
            $(echo "$order" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            financial_status = EXCLUDED.financial_status,
            fulfillment_status = EXCLUDED.fulfillment_status,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"

        # Sync line items
        echo "$order" | jq -c '.line_items[]' 2>/dev/null | while read -r item; do
            plugin_db_query "INSERT INTO shopify_order_items (
                id, order_id, product_id, variant_id, title, variant_title,
                sku, vendor, quantity, price, total_discount,
                fulfillment_status, fulfillable_quantity, grams,
                requires_shipping, taxable, gift_card
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
                $(echo "$item" | jq '.fulfillable_quantity'),
                $(echo "$item" | jq '.grams'),
                $(echo "$item" | jq '.requires_shipping'),
                $(echo "$item" | jq '.taxable'),
                $(echo "$item" | jq '.gift_card')
            ) ON CONFLICT (id) DO UPDATE SET
                fulfillment_status = EXCLUDED.fulfillment_status,
                fulfillable_quantity = EXCLUDED.fulfillable_quantity,
                synced_at = NOW()"
        done
    done

    plugin_log "success" "Orders synced"
}

main() {
    local initial=false
    local products_only=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --initial) initial=true; shift ;;
            --products-only) products_only=true; shift ;;
            *) shift ;;
        esac
    done

    plugin_log "info" "Starting Shopify sync..."

    if [[ -z "${SHOPIFY_STORE:-}" ]] || [[ -z "${SHOPIFY_ACCESS_TOKEN:-}" ]]; then
        plugin_log "error" "SHOPIFY_STORE and SHOPIFY_ACCESS_TOKEN required"
        return 1
    fi

    local shop_id
    shop_id=$(sync_shop)

    if [[ -z "$shop_id" ]]; then
        plugin_log "error" "Failed to sync shop"
        return 1
    fi

    sync_products "$shop_id"

    if [[ "$products_only" != "true" ]]; then
        sync_customers "$shop_id"
        sync_orders "$shop_id"
    fi

    plugin_set_meta "shopify" "last_sync" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    plugin_log "success" "Shopify sync complete"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
