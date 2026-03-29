#!/bin/bash
# =============================================================================
# Stripe customer.updated Event Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

handle_customer_updated() {
    local payload="$1"

    local customer_data
    customer_data=$(printf '%s' "$payload" | grep -o '"object":{[^}]*}' | head -1)

    local customer_id email name phone description currency balance delinquent

    customer_id=$(plugin_json_get "$customer_data" "id")
    email=$(plugin_json_get "$customer_data" "email")
    name=$(plugin_json_get "$customer_data" "name")
    phone=$(plugin_json_get "$customer_data" "phone")
    description=$(plugin_json_get "$customer_data" "description")
    currency=$(plugin_json_get "$customer_data" "currency")
    balance=$(printf '%s' "$customer_data" | grep -o '"balance":[0-9-]*' | sed 's/"balance"://')
    delinquent=$(printf '%s' "$customer_data" | grep -o '"delinquent":[a-z]*' | sed 's/"delinquent"://')

    plugin_info "Updating customer: $customer_id ($email)"

    # Escape values
    email=$(printf '%s' "$email" | sed "s/'/''/g")
    name=$(printf '%s' "$name" | sed "s/'/''/g")
    description=$(printf '%s' "$description" | sed "s/'/''/g")

    plugin_db_query "
        UPDATE stripe_customers SET
            email = $([ -n "$email" ] && echo "'$email'" || echo "email"),
            name = $([ -n "$name" ] && echo "'$name'" || echo "name"),
            phone = $([ -n "$phone" ] && echo "'$phone'" || echo "phone"),
            description = $([ -n "$description" ] && echo "'$description'" || echo "description"),
            currency = $([ -n "$currency" ] && echo "'$currency'" || echo "currency"),
            balance = $([ -n "$balance" ] && echo "$balance" || echo "balance"),
            delinquent = $([ -n "$delinquent" ] && echo "$delinquent" || echo "delinquent"),
            updated_at = NOW(),
            synced_at = NOW()
        WHERE id = '$customer_id';
    " >/dev/null

    plugin_success "Customer updated: $customer_id"
}

handle_customer_updated "$@"
