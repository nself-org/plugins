package internal

import (
	"context"
	"encoding/json"
)

// ============================================================================
// List / Get / Count operations for API endpoints
// ============================================================================

// ListCustomers returns paginated customers scoped by source_account_id.
func (db *DB) ListCustomers(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_customers", "deleted_at IS NULL", limit, offset)
}

func (db *DB) CountCustomers(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_customers", "deleted_at IS NULL")
}

func (db *DB) GetCustomer(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_customers", id)
}

func (db *DB) ListProducts(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_products", "deleted_at IS NULL", limit, offset)
}

func (db *DB) CountProducts(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_products", "deleted_at IS NULL")
}

func (db *DB) GetProduct(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_products", id)
}

func (db *DB) ListPrices(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_prices", "deleted_at IS NULL", limit, offset)
}

func (db *DB) CountPrices(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_prices", "deleted_at IS NULL")
}

func (db *DB) GetPrice(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_prices", id)
}

func (db *DB) ListSubscriptions(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_subscriptions", "", limit, offset)
}

func (db *DB) CountSubscriptions(ctx context.Context, status string) (int64, error) {
	if status != "" {
		var count int64
		err := db.Pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM np_stripe_subscriptions WHERE source_account_id = $1 AND status = $2",
			db.SourceAccountID, status,
		).Scan(&count)
		return count, err
	}
	return db.countTable(ctx, "np_stripe_subscriptions", "")
}

func (db *DB) GetSubscription(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_subscriptions", id)
}

func (db *DB) ListInvoices(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_invoices", "", limit, offset)
}

func (db *DB) CountInvoices(ctx context.Context, status string) (int64, error) {
	if status != "" {
		var count int64
		err := db.Pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM np_stripe_invoices WHERE source_account_id = $1 AND status = $2",
			db.SourceAccountID, status,
		).Scan(&count)
		return count, err
	}
	return db.countTable(ctx, "np_stripe_invoices", "")
}

func (db *DB) GetInvoice(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_invoices", id)
}

func (db *DB) ListPaymentIntents(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_payment_intents", "", limit, offset)
}

func (db *DB) CountPaymentIntents(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_payment_intents", "")
}

func (db *DB) GetPaymentIntent(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_payment_intents", id)
}

func (db *DB) ListCharges(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_charges", "", limit, offset)
}

func (db *DB) CountCharges(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_charges", "")
}

func (db *DB) GetCharge(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_charges", id)
}

func (db *DB) ListRefunds(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_refunds", "", limit, offset)
}

func (db *DB) CountRefunds(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_refunds", "")
}

func (db *DB) GetRefund(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_refunds", id)
}

func (db *DB) ListCoupons(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_coupons", "deleted_at IS NULL", limit, offset)
}

func (db *DB) CountCoupons(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_coupons", "deleted_at IS NULL")
}

func (db *DB) GetCoupon(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_coupons", id)
}

func (db *DB) ListBalanceTransactions(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_balance_transactions", "", limit, offset)
}

func (db *DB) CountBalanceTransactions(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_balance_transactions", "")
}

func (db *DB) GetBalanceTransaction(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_balance_transactions", id)
}

func (db *DB) ListPayouts(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_payouts", "", limit, offset)
}

func (db *DB) CountPayouts(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_payouts", "")
}

func (db *DB) GetPayout(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_payouts", id)
}

func (db *DB) ListDisputes(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_disputes", "", limit, offset)
}

func (db *DB) CountDisputes(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_disputes", "")
}

func (db *DB) GetDispute(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_disputes", id)
}

func (db *DB) ListEvents(ctx context.Context, eventType string, limit, offset int) ([]json.RawMessage, error) {
	if eventType != "" {
		query := "SELECT row_to_json(t) FROM np_stripe_webhook_events t WHERE source_account_id = $1 AND type = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4"
		rows, err := db.Pool.Query(ctx, query, db.SourceAccountID, eventType, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return collectJSONRows(rows)
	}
	return db.listRows(ctx, "np_stripe_webhook_events", "", limit, offset)
}

func (db *DB) ListCheckoutSessions(ctx context.Context, limit, offset int) ([]json.RawMessage, error) {
	return db.listRows(ctx, "np_stripe_checkout_sessions", "", limit, offset)
}

func (db *DB) CountCheckoutSessions(ctx context.Context) (int64, error) {
	return db.countTable(ctx, "np_stripe_checkout_sessions", "")
}

func (db *DB) GetCheckoutSession(ctx context.Context, id string) (json.RawMessage, error) {
	return db.getRow(ctx, "np_stripe_checkout_sessions", id)
}

