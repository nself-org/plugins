#!/bin/bash
# =============================================================================
# Stripe customer.created Event Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

handle_customer_created() {
    local payload="$1"

    # Extract customer data from webhook payload
    local customer_data
    customer_data=$(printf '%s' "$payload" | grep -o '"object":{[^}]*}' | head -1)

    local customer_id email name phone description currency created

    customer_id=$(plugin_json_get "$customer_data" "id")
    email=$(plugin_json_get "$customer_data" "email")
    name=$(plugin_json_get "$customer_data" "name")
    phone=$(plugin_json_get "$customer_data" "phone")
    description=$(plugin_json_get "$customer_data" "description")
    currency=$(plugin_json_get "$customer_data" "currency")
    created=$(printf '%s' "$customer_data" | grep -o '"created":[0-9]*' | sed 's/"created"://')

    plugin_info "Creating customer: $customer_id ($email)"

    # Escape values for SQL
    email=$(printf '%s' "$email" | sed "s/'/''/g")
    name=$(printf '%s' "$name" | sed "s/'/''/g")
    description=$(printf '%s' "$description" | sed "s/'/''/g")

    # Insert customer
    plugin_db_query "
        INSERT INTO stripe_customers (
            id, email, name, phone, description, currency, created_at, synced_at
        ) VALUES (
            '$customer_id',
            $([ -n "$email" ] && echo "'$email'" || echo "NULL"),
            $([ -n "$name" ] && echo "'$name'" || echo "NULL"),
            $([ -n "$phone" ] && echo "'$phone'" || echo "NULL"),
            $([ -n "$description" ] && echo "'$description'" || echo "NULL"),
            $([ -n "$currency" ] && echo "'$currency'" || echo "NULL"),
            to_timestamp($created),
            NOW()
        )
        ON CONFLICT (id) DO UPDATE SET
            email = EXCLUDED.email,
            name = EXCLUDED.name,
            phone = EXCLUDED.phone,
            description = EXCLUDED.description,
            currency = EXCLUDED.currency,
            synced_at = NOW();
    " >/dev/null

    plugin_success "Customer created: $customer_id"
}

handle_customer_created "$@"
