#!/usr/bin/env bash
# Shopify Plugin - Webhook Handler

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

verify_signature() {
    local payload="$1"
    local signature="$2"

    if [[ -z "${SHOPIFY_WEBHOOK_SECRET:-}" ]]; then
        plugin_log "warning" "SHOPIFY_WEBHOOK_SECRET not set, skipping verification"
        return 0
    fi

    if [[ -z "$signature" ]]; then
        plugin_log "error" "No signature provided"
        return 1
    fi

    local expected
    expected=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$SHOPIFY_WEBHOOK_SECRET" -binary | base64)

    if [[ "$signature" != "$expected" ]]; then
        plugin_log "error" "Invalid webhook signature"
        return 1
    fi

    return 0
}

process_event() {
    local event_id="$1"
    local topic="$2"
    local payload="$3"

    plugin_log "info" "Processing event: $topic"

    # Convert topic to filename (orders/create -> orders_create)
    local handler_name
    handler_name=$(echo "$topic" | tr '/' '_')
    local handler_script="${PLUGIN_DIR}/webhooks/events/${handler_name}.sh"
    local default_handler="${PLUGIN_DIR}/webhooks/events/default.sh"

    if [[ -x "$handler_script" ]]; then
        bash "$handler_script" "$event_id" "$payload"
    elif [[ -x "$default_handler" ]]; then
        bash "$default_handler" "$event_id" "$topic" "$payload"
    else
        plugin_log "warning" "No handler for topic: $topic"
    fi

    plugin_db_query "UPDATE shopify_webhook_events SET processed = true, processed_at = NOW() WHERE id = '$event_id'"
    return 0
}

handle_webhook() {
    local webhook_id="$1"
    local topic="$2"
    local signature="$3"
    local payload="$4"

    # If called with just event ID, load from database
    if [[ -n "$webhook_id" ]] && [[ -z "$topic" ]]; then
        local event_data
        event_data=$(plugin_db_query "SELECT topic, data FROM shopify_webhook_events WHERE id = '$webhook_id'" 2>/dev/null)
        if [[ -n "$event_data" ]]; then
            topic=$(echo "$event_data" | cut -d'|' -f1)
            payload=$(echo "$event_data" | cut -d'|' -f2)
            process_event "$webhook_id" "$topic" "$payload"
            return $?
        fi
    fi

    if ! verify_signature "$payload" "$signature"; then
        return 1
    fi

    local shop_id shop_domain
    shop_id=$(echo "$payload" | jq -r '.id // empty')
    shop_domain="${HTTP_X_SHOPIFY_SHOP_DOMAIN:-}"

    plugin_db_query "INSERT INTO shopify_webhook_events (
        id, topic, shop_id, shop_domain, data, received_at
    ) VALUES (
        '$webhook_id',
        '$topic',
        $(if [[ -n "$shop_id" ]]; then echo "'$shop_id'"; else echo "NULL"; fi),
        $(if [[ -n "$shop_domain" ]]; then echo "'$shop_domain'"; else echo "NULL"; fi),
        '$(echo "$payload" | jq -c .)',
        NOW()
    ) ON CONFLICT (id) DO NOTHING"

    process_event "$webhook_id" "$topic" "$payload"
    return 0
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    if [[ $# -ge 1 ]]; then
        handle_webhook "$@"
    else
        webhook_id="${HTTP_X_SHOPIFY_WEBHOOK_ID:-$(uuidgen)}"
        topic="${HTTP_X_SHOPIFY_TOPIC:-}"
        signature="${HTTP_X_SHOPIFY_HMAC_SHA256:-}"
        payload=$(cat)

        if [[ -n "$topic" ]]; then
            handle_webhook "$webhook_id" "$topic" "$signature" "$payload"
        else
            echo "Error: Missing X-Shopify-Topic header" >&2
            exit 1
        fi
    fi
fi
