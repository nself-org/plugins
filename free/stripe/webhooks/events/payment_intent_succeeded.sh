#!/bin/bash
# =============================================================================
# Stripe payment_intent.succeeded Event Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

handle_payment_intent_succeeded() {
    local payload="$1"

    local pi_data
    pi_data=$(printf '%s' "$payload" | grep -o '"object":{[^}]*}' | head -1)

    local pi_id customer_id amount amount_received currency status payment_method_id created

    pi_id=$(plugin_json_get "$pi_data" "id")
    customer_id=$(plugin_json_get "$pi_data" "customer")
    amount=$(printf '%s' "$pi_data" | grep -o '"amount":[0-9]*' | head -1 | sed 's/"amount"://')
    amount_received=$(printf '%s' "$pi_data" | grep -o '"amount_received":[0-9]*' | sed 's/"amount_received"://')
    currency=$(plugin_json_get "$pi_data" "currency")
    status=$(plugin_json_get "$pi_data" "status")
    payment_method_id=$(plugin_json_get "$pi_data" "payment_method")
    created=$(printf '%s' "$pi_data" | grep -o '"created":[0-9]*' | sed 's/"created"://')

    plugin_info "Payment succeeded: $pi_id (amount: $amount_received $currency)"

    plugin_db_query "
        INSERT INTO stripe_payment_intents (
            id, customer_id, amount, amount_received, currency, status,
            payment_method_id, created_at, synced_at
        ) VALUES (
            '$pi_id',
            $([ -n "$customer_id" ] && echo "'$customer_id'" || echo "NULL"),
            ${amount:-0},
            ${amount_received:-0},
            '$currency',
            '$status',
            $([ -n "$payment_method_id" ] && echo "'$payment_method_id'" || echo "NULL"),
            to_timestamp($created),
            NOW()
        )
        ON CONFLICT (id) DO UPDATE SET
            amount_received = EXCLUDED.amount_received,
            status = EXCLUDED.status,
            synced_at = NOW();
    " >/dev/null

    plugin_success "Payment intent recorded: $pi_id"
}

handle_payment_intent_succeeded "$@"
