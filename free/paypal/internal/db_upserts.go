package internal

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

