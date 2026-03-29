#!/bin/bash
# =============================================================================
# Stripe subscription.created Event Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

handle_subscription_created() {
    local payload="$1"

    local sub_data
    sub_data=$(printf '%s' "$payload" | grep -o '"object":{[^}]*}' | head -1)

    local sub_id customer_id status current_period_start current_period_end cancel_at_period_end created

    sub_id=$(plugin_json_get "$sub_data" "id")
    customer_id=$(plugin_json_get "$sub_data" "customer")
    status=$(plugin_json_get "$sub_data" "status")
    current_period_start=$(printf '%s' "$sub_data" | grep -o '"current_period_start":[0-9]*' | sed 's/"current_period_start"://')
    current_period_end=$(printf '%s' "$sub_data" | grep -o '"current_period_end":[0-9]*' | sed 's/"current_period_end"://')
    cancel_at_period_end=$(printf '%s' "$sub_data" | grep -o '"cancel_at_period_end":[a-z]*' | sed 's/"cancel_at_period_end"://')
    created=$(printf '%s' "$sub_data" | grep -o '"created":[0-9]*' | sed 's/"created"://')

    # Extract items
    local items
    items=$(printf '%s' "$payload" | grep -o '"items":{[^}]*}' | head -1 || echo '[]')

    plugin_info "Creating subscription: $sub_id (customer: $customer_id, status: $status)"

    # Escape items JSON
    items=$(printf '%s' "$items" | sed "s/'/''/g")

    plugin_db_query "
        INSERT INTO stripe_subscriptions (
            id, customer_id, status, current_period_start, current_period_end,
            cancel_at_period_end, items, created_at, synced_at
        ) VALUES (
            '$sub_id',
            '$customer_id',
            '$status',
            to_timestamp($current_period_start),
            to_timestamp($current_period_end),
            ${cancel_at_period_end:-false},
            '$items'::jsonb,
            to_timestamp($created),
            NOW()
        )
        ON CONFLICT (id) DO UPDATE SET
            status = EXCLUDED.status,
            current_period_start = EXCLUDED.current_period_start,
            current_period_end = EXCLUDED.current_period_end,
            cancel_at_period_end = EXCLUDED.cancel_at_period_end,
            items = EXCLUDED.items,
            synced_at = NOW();
    " >/dev/null

    plugin_success "Subscription created: $sub_id"
}

handle_subscription_created "$@"
