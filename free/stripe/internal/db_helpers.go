package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Internal helpers
// ============================================================================

// listRows returns rows as json.RawMessage from a table with source_account_id scoping.
func (db *DB) listRows(ctx context.Context, table, extraWhere string, limit, offset int) ([]json.RawMessage, error) {
	where := "source_account_id = $1"
	if extraWhere != "" {
		where += " AND " + extraWhere
	}
	query := fmt.Sprintf("SELECT row_to_json(t) FROM %s t WHERE %s ORDER BY created_at DESC LIMIT $2 OFFSET $3", table, where)
	rows, err := db.Pool.Query(ctx, query, db.SourceAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectJSONRows(rows)
}

func (db *DB) getRow(ctx context.Context, table, id string) (json.RawMessage, error) {
	query := fmt.Sprintf("SELECT row_to_json(t) FROM %s t WHERE id = $1 AND source_account_id = $2", table)
	var data json.RawMessage
	err := db.Pool.QueryRow(ctx, query, id, db.SourceAccountID).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return data, err
}

func collectJSONRows(rows pgx.Rows) ([]json.RawMessage, error) {
	var result []json.RawMessage
	for rows.Next() {
		var data json.RawMessage
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		result = append(result, data)
	}
	if result == nil {
		result = []json.RawMessage{}
	}
	return result, rows.Err()
}

func objectTypeToTable(objectType string) string {
	switch objectType {
	case "customer":
		return "np_stripe_customers"
	case "product":
		return "np_stripe_products"
	case "price":
		return "np_stripe_prices"
	case "coupon":
		return "np_stripe_coupons"
	case "promotion_code":
		return "np_stripe_promotion_codes"
	case "subscription":
		return "np_stripe_subscriptions"
	case "subscription_item":
		return "np_stripe_subscription_items"
	case "invoice":
		return "np_stripe_invoices"
	case "invoiceitem":
		return "np_stripe_invoice_items"
	case "charge":
		return "np_stripe_charges"
	case "refund":
		return "np_stripe_refunds"
	case "dispute":
		return "np_stripe_disputes"
	case "payment_intent":
		return "np_stripe_payment_intents"
	case "setup_intent":
		return "np_stripe_setup_intents"
	case "payment_method":
		return "np_stripe_payment_methods"
	case "balance_transaction":
		return "np_stripe_balance_transactions"
	case "payout":
		return "np_stripe_payouts"
	case "checkout.session", "checkout_session":
		return "np_stripe_checkout_sessions"
	case "tax_rate":
		return "np_stripe_tax_rates"
	case "tax_id":
		return "np_stripe_tax_ids"
	default:
		return ""
	}
}

func nullStr(ns NullString) interface{} {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullTime(nt NullTime) interface{} {
	if !nt.Valid {
		return nil
	}
	return nt.Time
}

