package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Query functions ---------------------------------------------------------

// ListTransactions returns transactions with optional limit and offset.
func ListTransactions(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Transaction, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, paypal_id, type, status, amount, currency, fee, net, payer_email, payer_name, description, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_transactions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.PayPalID, &t.Type, &t.Status, &t.Amount, &t.Currency, &t.Fee, &t.Net,
			&t.PayerEmail, &t.PayerName, &t.Description, &t.CreatedAt, &t.UpdatedAt, &t.SourceAccountID, &t.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// GetTransaction returns a single transaction by ID.
func GetTransaction(ctx context.Context, pool *pgxpool.Pool, id string) (*Transaction, error) {
	var t Transaction
	err := pool.QueryRow(ctx, `
		SELECT id, paypal_id, type, status, amount, currency, fee, net, payer_email, payer_name, description, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_transactions WHERE id = $1
	`, id).Scan(&t.ID, &t.PayPalID, &t.Type, &t.Status, &t.Amount, &t.Currency, &t.Fee, &t.Net,
		&t.PayerEmail, &t.PayerName, &t.Description, &t.CreatedAt, &t.UpdatedAt, &t.SourceAccountID, &t.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListOrders returns orders with optional limit and offset.
func ListOrders(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Order, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, paypal_id, status, intent, purchase_units, payer, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_orders
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.PayPalID, &o.Status, &o.Intent, &o.PurchaseUnits, &o.Payer,
			&o.CreatedAt, &o.UpdatedAt, &o.SourceAccountID, &o.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, o)
	}
	return results, rows.Err()
}

// GetOrder returns a single order by ID.
func GetOrder(ctx context.Context, pool *pgxpool.Pool, id string) (*Order, error) {
	var o Order
	err := pool.QueryRow(ctx, `
		SELECT id, paypal_id, status, intent, purchase_units, payer, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_orders WHERE id = $1
	`, id).Scan(&o.ID, &o.PayPalID, &o.Status, &o.Intent, &o.PurchaseUnits, &o.Payer,
		&o.CreatedAt, &o.UpdatedAt, &o.SourceAccountID, &o.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListSubscriptions returns subscriptions with optional limit and offset.
func ListSubscriptions(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Subscription, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, paypal_id, plan_id, status, subscriber, start_time, billing_info, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_subscriptions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.PayPalID, &s.PlanID, &s.Status, &s.Subscriber, &s.StartTime,
			&s.BillingInfo, &s.CreatedAt, &s.UpdatedAt, &s.SourceAccountID, &s.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetSubscription returns a single subscription by ID.
func GetSubscription(ctx context.Context, pool *pgxpool.Pool, id string) (*Subscription, error) {
	var s Subscription
	err := pool.QueryRow(ctx, `
		SELECT id, paypal_id, plan_id, status, subscriber, start_time, billing_info, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_subscriptions WHERE id = $1
	`, id).Scan(&s.ID, &s.PayPalID, &s.PlanID, &s.Status, &s.Subscriber, &s.StartTime,
		&s.BillingInfo, &s.CreatedAt, &s.UpdatedAt, &s.SourceAccountID, &s.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListDisputes returns disputes with optional limit and offset.
func ListDisputes(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Dispute, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, paypal_id, reason, status, dispute_amount, dispute_currency, messages, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_disputes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Dispute
	for rows.Next() {
		var d Dispute
		if err := rows.Scan(&d.ID, &d.PayPalID, &d.Reason, &d.Status, &d.DisputeAmount, &d.DisputeCurrency,
			&d.Messages, &d.CreatedAt, &d.UpdatedAt, &d.SourceAccountID, &d.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// ListInvoices returns invoices with optional limit and offset.
func ListInvoices(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Invoice, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, paypal_id, status, detail, amount, currency, due_date, invoicer, created_at, updated_at, source_account_id, synced_at
		FROM np_paypal_invoices
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.PayPalID, &inv.Status, &inv.Detail, &inv.Amount, &inv.Currency,
			&inv.DueDate, &inv.Invoicer, &inv.CreatedAt, &inv.UpdatedAt, &inv.SourceAccountID, &inv.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, inv)
	}
	return results, rows.Err()
}

// ListWebhookEvents returns recent webhook events.
func ListWebhookEvents(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]WebhookEvent, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, paypal_event_id, event_type, resource_type, resource, summary, create_time, processed, source_account_id
		FROM np_paypal_webhook_events
		ORDER BY create_time DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.PayPalEventID, &e.EventType, &e.ResourceType, &e.Resource,
			&e.Summary, &e.CreateTime, &e.Processed, &e.SourceAccountID); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// GetSyncStats returns record counts for each table.
func GetSyncStats(ctx context.Context, pool *pgxpool.Pool) (*SyncStats, error) {
	stats := &SyncStats{}

	tables := []struct {
		name string
		dest *int
	}{
		{"np_paypal_transactions", &stats.Transactions},
		{"np_paypal_orders", &stats.Orders},
		{"np_paypal_captures", &stats.Captures},
		{"np_paypal_authorizations", &stats.Authorizations},
		{"np_paypal_refunds", &stats.Refunds},
		{"np_paypal_subscriptions", &stats.Subscriptions},
		{"np_paypal_subscription_plans", &stats.SubscriptionPlans},
		{"np_paypal_products", &stats.Products},
		{"np_paypal_disputes", &stats.Disputes},
		{"np_paypal_payouts", &stats.Payouts},
		{"np_paypal_invoices", &stats.Invoices},
		{"np_paypal_payers", &stats.Payers},
		{"np_paypal_balances", &stats.Balances},
		{"np_paypal_webhook_events", &stats.WebhookEvents},
	}

	for _, t := range tables {
		var count int
		err := pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", t.name)).Scan(&count)
		if err != nil {
			continue
		}
		*t.dest = count
	}

	err := pool.QueryRow(ctx, "SELECT MAX(synced_at) FROM np_paypal_transactions").Scan(&stats.LastSyncedAt)
	if err != nil {
		stats.LastSyncedAt = nil
	}

	return stats, nil
}

// ensureJSON returns the raw JSON if non-nil and non-empty, otherwise returns the fallback string as raw JSON.
func ensureJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}

