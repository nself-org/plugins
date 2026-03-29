#!/bin/bash
# =============================================================================
# Stripe invoice.paid Event Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

handle_invoice_paid() {
    local payload="$1"

    local invoice_data
    invoice_data=$(printf '%s' "$payload" | grep -o '"object":{[^}]*}' | head -1)

    local invoice_id customer_id subscription_id amount_paid currency status

    invoice_id=$(plugin_json_get "$invoice_data" "id")
    customer_id=$(plugin_json_get "$invoice_data" "customer")
    subscription_id=$(plugin_json_get "$invoice_data" "subscription")
    amount_paid=$(printf '%s' "$invoice_data" | grep -o '"amount_paid":[0-9]*' | sed 's/"amount_paid"://')
    currency=$(plugin_json_get "$invoice_data" "currency")
    status=$(plugin_json_get "$invoice_data" "status")

    plugin_info "Invoice paid: $invoice_id (amount: $amount_paid $currency)"

    # Update or insert invoice
    plugin_db_query "
        INSERT INTO stripe_invoices (
            id, customer_id, subscription_id, amount_paid, currency, status, paid, synced_at
        ) VALUES (
            '$invoice_id',
            '$customer_id',
            $([ -n "$subscription_id" ] && echo "'$subscription_id'" || echo "NULL"),
            $amount_paid,
            '$currency',
            '$status',
            TRUE,
            NOW()
        )
        ON CONFLICT (id) DO UPDATE SET
            amount_paid = EXCLUDED.amount_paid,
            status = EXCLUDED.status,
            paid = TRUE,
            synced_at = NOW();
    " >/dev/null

    plugin_success "Invoice marked as paid: $invoice_id"
}

handle_invoice_paid "$@"
