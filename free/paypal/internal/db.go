package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate creates all 14 tables and their indexes if they do not exist.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_paypal_transactions (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			type            TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT '',
			amount          NUMERIC(20,2) NOT NULL DEFAULT 0,
			currency        TEXT NOT NULL DEFAULT 'USD',
			fee             NUMERIC(20,2),
			net             NUMERIC(20,2),
			payer_email     TEXT,
			payer_name      TEXT,
			description     TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_transactions_paypal_id
			ON np_paypal_transactions (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_transactions_status
			ON np_paypal_transactions (status);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_transactions_created
			ON np_paypal_transactions (created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_transactions_account
			ON np_paypal_transactions (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_orders (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT '',
			intent          TEXT NOT NULL DEFAULT '',
			purchase_units  JSONB NOT NULL DEFAULT '[]',
			payer           JSONB NOT NULL DEFAULT '{}',
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_orders_paypal_id
			ON np_paypal_orders (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_orders_status
			ON np_paypal_orders (status);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_orders_created
			ON np_paypal_orders (created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_orders_account
			ON np_paypal_orders (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_captures (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			order_id        TEXT,
			status          TEXT NOT NULL DEFAULT '',
			amount          NUMERIC(20,2) NOT NULL DEFAULT 0,
			currency        TEXT NOT NULL DEFAULT 'USD',
			final_capture   BOOLEAN NOT NULL DEFAULT false,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_captures_paypal_id
			ON np_paypal_captures (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_captures_order
			ON np_paypal_captures (order_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_captures_account
			ON np_paypal_captures (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_authorizations (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			order_id        TEXT,
			status          TEXT NOT NULL DEFAULT '',
			amount          NUMERIC(20,2) NOT NULL DEFAULT 0,
			currency        TEXT NOT NULL DEFAULT 'USD',
			expiration_time TIMESTAMPTZ,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_authorizations_paypal_id
			ON np_paypal_authorizations (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_authorizations_order
			ON np_paypal_authorizations (order_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_authorizations_account
			ON np_paypal_authorizations (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_refunds (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			capture_id      TEXT,
			status          TEXT NOT NULL DEFAULT '',
			amount          NUMERIC(20,2) NOT NULL DEFAULT 0,
			currency        TEXT NOT NULL DEFAULT 'USD',
			reason          TEXT,
			created_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_refunds_paypal_id
			ON np_paypal_refunds (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_refunds_capture
			ON np_paypal_refunds (capture_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_refunds_account
			ON np_paypal_refunds (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_subscriptions (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			plan_id         TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT '',
			subscriber      JSONB NOT NULL DEFAULT '{}',
			start_time      TIMESTAMPTZ,
			billing_info    JSONB NOT NULL DEFAULT '{}',
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_subscriptions_paypal_id
			ON np_paypal_subscriptions (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_subscriptions_status
			ON np_paypal_subscriptions (status);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_subscriptions_plan
			ON np_paypal_subscriptions (plan_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_subscriptions_account
			ON np_paypal_subscriptions (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_subscription_plans (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			product_id      TEXT NOT NULL DEFAULT '',
			name            TEXT NOT NULL DEFAULT '',
			description     TEXT,
			status          TEXT NOT NULL DEFAULT '',
			billing_cycles  JSONB NOT NULL DEFAULT '[]',
			payment_preferences JSONB NOT NULL DEFAULT '{}',
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_sub_plans_paypal_id
			ON np_paypal_subscription_plans (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_sub_plans_product
			ON np_paypal_subscription_plans (product_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_sub_plans_account
			ON np_paypal_subscription_plans (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_products (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			name            TEXT NOT NULL DEFAULT '',
			description     TEXT,
			type            TEXT NOT NULL DEFAULT '',
			category        TEXT,
			image_url       TEXT,
			home_url        TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_products_paypal_id
			ON np_paypal_products (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_products_account
			ON np_paypal_products (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_disputes (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			reason          TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT '',
			dispute_amount  NUMERIC(20,2) NOT NULL DEFAULT 0,
			dispute_currency TEXT NOT NULL DEFAULT 'USD',
			messages        JSONB NOT NULL DEFAULT '[]',
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_disputes_paypal_id
			ON np_paypal_disputes (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_disputes_status
			ON np_paypal_disputes (status);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_disputes_created
			ON np_paypal_disputes (created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_disputes_account
			ON np_paypal_disputes (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_payouts (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			batch_id        TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT '',
			amount          NUMERIC(20,2),
			currency        TEXT,
			recipient_type  TEXT,
			receiver        TEXT,
			sender_item_id  TEXT,
			created_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_payouts_paypal_id
			ON np_paypal_payouts (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_payouts_batch
			ON np_paypal_payouts (batch_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_payouts_account
			ON np_paypal_payouts (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_invoices (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT '',
			detail          JSONB NOT NULL DEFAULT '{}',
			amount          NUMERIC(20,2),
			currency        TEXT,
			due_date        TEXT,
			invoicer        JSONB NOT NULL DEFAULT '{}',
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_invoices_paypal_id
			ON np_paypal_invoices (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_invoices_status
			ON np_paypal_invoices (status);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_invoices_account
			ON np_paypal_invoices (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_payers (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_id       TEXT NOT NULL,
			email           TEXT,
			name            TEXT,
			phone           TEXT,
			address         JSONB NOT NULL DEFAULT '{}',
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_payers_paypal_id
			ON np_paypal_payers (paypal_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_payers_email
			ON np_paypal_payers (email);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_payers_account
			ON np_paypal_payers (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_balances (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			currency        TEXT NOT NULL DEFAULT 'USD',
			total_balance   NUMERIC(20,2),
			available_balance NUMERIC(20,2),
			withheld_balance NUMERIC(20,2),
			recorded_at     TIMESTAMPTZ DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary'
		);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_balances_currency
			ON np_paypal_balances (currency, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_balances_account
			ON np_paypal_balances (source_account_id);

		CREATE TABLE IF NOT EXISTS np_paypal_webhook_events (
			id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
			paypal_event_id TEXT NOT NULL,
			event_type      TEXT NOT NULL DEFAULT '',
			resource_type   TEXT NOT NULL DEFAULT '',
			resource        JSONB NOT NULL DEFAULT '{}',
			summary         TEXT,
			create_time     TIMESTAMPTZ,
			processed       BOOLEAN NOT NULL DEFAULT false,
			source_account_id TEXT NOT NULL DEFAULT 'primary'
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_paypal_webhook_events_event_id
			ON np_paypal_webhook_events (paypal_event_id);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_webhook_events_type
			ON np_paypal_webhook_events (event_type);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_webhook_events_created
			ON np_paypal_webhook_events (create_time DESC);
		CREATE INDEX IF NOT EXISTS idx_np_paypal_webhook_events_account
			ON np_paypal_webhook_events (source_account_id);
	`)
	return err
}

// --- Upsert functions --------------------------------------------------------

// UpsertTransaction inserts or updates a transaction by its paypal_id.
func UpsertTransaction(ctx context.Context, pool *pgxpool.Pool, t *Transaction) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_transactions (paypal_id, type, status, amount, currency, fee, net, payer_email, payer_name, description, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			type = EXCLUDED.type,
			status = EXCLUDED.status,
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			fee = EXCLUDED.fee,
			net = EXCLUDED.net,
			payer_email = EXCLUDED.payer_email,
			payer_name = EXCLUDED.payer_name,
			description = EXCLUDED.description,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, t.PayPalID, t.Type, t.Status, t.Amount, t.Currency, t.Fee, t.Net,
		t.PayerEmail, t.PayerName, t.Description, t.CreatedAt, t.UpdatedAt, t.SourceAccountID)
	return err
}

// UpsertOrder inserts or updates an order by its paypal_id.
func UpsertOrder(ctx context.Context, pool *pgxpool.Pool, o *Order) error {
	pu := ensureJSON(o.PurchaseUnits, "[]")
	p := ensureJSON(o.Payer, "{}")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_orders (paypal_id, status, intent, purchase_units, payer, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			status = EXCLUDED.status,
			intent = EXCLUDED.intent,
			purchase_units = EXCLUDED.purchase_units,
			payer = EXCLUDED.payer,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, o.PayPalID, o.Status, o.Intent, pu, p, o.CreatedAt, o.UpdatedAt, o.SourceAccountID)
	return err
}

// UpsertCapture inserts or updates a capture by its paypal_id.
func UpsertCapture(ctx context.Context, pool *pgxpool.Pool, c *Capture) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_captures (paypal_id, order_id, status, amount, currency, final_capture, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			order_id = EXCLUDED.order_id,
			status = EXCLUDED.status,
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			final_capture = EXCLUDED.final_capture,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, c.PayPalID, c.OrderID, c.Status, c.Amount, c.Currency, c.FinalCapture, c.CreatedAt, c.UpdatedAt, c.SourceAccountID)
	return err
}

// UpsertAuthorization inserts or updates an authorization by its paypal_id.
func UpsertAuthorization(ctx context.Context, pool *pgxpool.Pool, a *Authorization) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_authorizations (paypal_id, order_id, status, amount, currency, expiration_time, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			order_id = EXCLUDED.order_id,
			status = EXCLUDED.status,
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			expiration_time = EXCLUDED.expiration_time,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, a.PayPalID, a.OrderID, a.Status, a.Amount, a.Currency, a.ExpirationTime, a.CreatedAt, a.UpdatedAt, a.SourceAccountID)
	return err
}

// UpsertRefund inserts or updates a refund by its paypal_id.
func UpsertRefund(ctx context.Context, pool *pgxpool.Pool, r *Refund) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_refunds (paypal_id, capture_id, status, amount, currency, reason, created_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			capture_id = EXCLUDED.capture_id,
			status = EXCLUDED.status,
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			reason = EXCLUDED.reason,
			synced_at = NOW()
	`, r.PayPalID, r.CaptureID, r.Status, r.Amount, r.Currency, r.Reason, r.CreatedAt, r.SourceAccountID)
	return err
}

// UpsertSubscription inserts or updates a subscription by its paypal_id.
func UpsertSubscription(ctx context.Context, pool *pgxpool.Pool, s *Subscription) error {
	sub := ensureJSON(s.Subscriber, "{}")
	bi := ensureJSON(s.BillingInfo, "{}")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_subscriptions (paypal_id, plan_id, status, subscriber, start_time, billing_info, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			plan_id = EXCLUDED.plan_id,
			status = EXCLUDED.status,
			subscriber = EXCLUDED.subscriber,
			start_time = EXCLUDED.start_time,
			billing_info = EXCLUDED.billing_info,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, s.PayPalID, s.PlanID, s.Status, sub, s.StartTime, bi, s.CreatedAt, s.UpdatedAt, s.SourceAccountID)
	return err
}

// UpsertSubscriptionPlan inserts or updates a subscription plan by its paypal_id.
func UpsertSubscriptionPlan(ctx context.Context, pool *pgxpool.Pool, sp *SubscriptionPlan) error {
	bc := ensureJSON(sp.BillingCycles, "[]")
	pp := ensureJSON(sp.PaymentPreferences, "{}")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_subscription_plans (paypal_id, product_id, name, description, status, billing_cycles, payment_preferences, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			product_id = EXCLUDED.product_id,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			status = EXCLUDED.status,
			billing_cycles = EXCLUDED.billing_cycles,
			payment_preferences = EXCLUDED.payment_preferences,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, sp.PayPalID, sp.ProductID, sp.Name, sp.Description, sp.Status, bc, pp, sp.CreatedAt, sp.UpdatedAt, sp.SourceAccountID)
	return err
}

// UpsertProduct inserts or updates a product by its paypal_id.
func UpsertProduct(ctx context.Context, pool *pgxpool.Pool, p *Product) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_products (paypal_id, name, description, type, category, image_url, home_url, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			type = EXCLUDED.type,
			category = EXCLUDED.category,
			image_url = EXCLUDED.image_url,
			home_url = EXCLUDED.home_url,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, p.PayPalID, p.Name, p.Description, p.Type, p.Category, p.ImageURL, p.HomeURL, p.CreatedAt, p.UpdatedAt, p.SourceAccountID)
	return err
}

// UpsertDispute inserts or updates a dispute by its paypal_id.
func UpsertDispute(ctx context.Context, pool *pgxpool.Pool, d *Dispute) error {
	msgs := ensureJSON(d.Messages, "[]")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_disputes (paypal_id, reason, status, dispute_amount, dispute_currency, messages, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			reason = EXCLUDED.reason,
			status = EXCLUDED.status,
			dispute_amount = EXCLUDED.dispute_amount,
			dispute_currency = EXCLUDED.dispute_currency,
			messages = EXCLUDED.messages,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, d.PayPalID, d.Reason, d.Status, d.DisputeAmount, d.DisputeCurrency, msgs, d.CreatedAt, d.UpdatedAt, d.SourceAccountID)
	return err
}

// UpsertPayout inserts or updates a payout by its paypal_id.
func UpsertPayout(ctx context.Context, pool *pgxpool.Pool, p *Payout) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_payouts (paypal_id, batch_id, status, amount, currency, recipient_type, receiver, sender_item_id, created_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			batch_id = EXCLUDED.batch_id,
			status = EXCLUDED.status,
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			recipient_type = EXCLUDED.recipient_type,
			receiver = EXCLUDED.receiver,
			sender_item_id = EXCLUDED.sender_item_id,
			synced_at = NOW()
	`, p.PayPalID, p.BatchID, p.Status, p.Amount, p.Currency, p.RecipientType, p.Receiver, p.SenderItemID, p.CreatedAt, p.SourceAccountID)
	return err
}

// UpsertInvoice inserts or updates an invoice by its paypal_id.
func UpsertInvoice(ctx context.Context, pool *pgxpool.Pool, inv *Invoice) error {
	detail := ensureJSON(inv.Detail, "{}")
	invoicer := ensureJSON(inv.Invoicer, "{}")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_invoices (paypal_id, status, detail, amount, currency, due_date, invoicer, created_at, updated_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			status = EXCLUDED.status,
			detail = EXCLUDED.detail,
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			due_date = EXCLUDED.due_date,
			invoicer = EXCLUDED.invoicer,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, inv.PayPalID, inv.Status, detail, inv.Amount, inv.Currency, inv.DueDate, invoicer, inv.CreatedAt, inv.UpdatedAt, inv.SourceAccountID)
	return err
}

// UpsertPayer inserts or updates a payer by their paypal_id.
func UpsertPayer(ctx context.Context, pool *pgxpool.Pool, p *Payer) error {
	addr := ensureJSON(p.Address, "{}")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_payers (paypal_id, email, name, phone, address, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (paypal_id, source_account_id) DO UPDATE SET
			email = COALESCE(EXCLUDED.email, np_paypal_payers.email),
			name = COALESCE(EXCLUDED.name, np_paypal_payers.name),
			phone = COALESCE(EXCLUDED.phone, np_paypal_payers.phone),
			address = EXCLUDED.address,
			synced_at = NOW()
	`, p.PayPalID, p.Email, p.Name, p.Phone, addr, p.SourceAccountID)
	return err
}

// InsertWebhookEvent inserts a webhook event record.
func InsertWebhookEvent(ctx context.Context, pool *pgxpool.Pool, e *WebhookEvent) error {
	resource := ensureJSON(e.Resource, "{}")
	_, err := pool.Exec(ctx, `
		INSERT INTO np_paypal_webhook_events (paypal_event_id, event_type, resource_type, resource, summary, create_time, processed, source_account_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (paypal_event_id) DO NOTHING
	`, e.PayPalEventID, e.EventType, e.ResourceType, resource, e.Summary, e.CreateTime, e.Processed, e.SourceAccountID)
	return err
}

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
