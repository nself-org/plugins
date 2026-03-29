package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with source_account_id scoping.
type DB struct {
	Pool            *pgxpool.Pool
	SourceAccountID string
}

// NewDB creates a new DB instance with the given pool and default source account "primary".
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{Pool: pool, SourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to the given source_account_id.
func (db *DB) ForSourceAccount(sourceAccountID string) *DB {
	id := sourceAccountID
	if id == "" {
		id = "primary"
	}
	return &DB{Pool: db.Pool, SourceAccountID: id}
}

// InitSchema creates all 23 np_stripe_* tables, indexes, and analytics views.
func (db *DB) InitSchema(ctx context.Context) error {
	log.Println("[stripe:db] Initializing schema...")

	_, err := db.Pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("[stripe:db] Schema initialized (23 tables, 7 views)")
	return nil
}

// countTable returns COUNT(*) for a table scoped by source_account_id with optional extra WHERE.
func (db *DB) countTable(ctx context.Context, table string, extraWhere string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE source_account_id = $1", table)
	if extraWhere != "" {
		query += " AND " + extraWhere
	}
	var count int64
	err := db.Pool.QueryRow(ctx, query, db.SourceAccountID).Scan(&count)
	return count, err
}

// GetStats returns aggregate counts for all tables scoped by source_account_id.
func (db *DB) GetStats(ctx context.Context) (*SyncStats, error) {
	stats := &SyncStats{}
	var err error

	stats.Customers, err = db.countTable(ctx, "np_stripe_customers", "deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	stats.Products, err = db.countTable(ctx, "np_stripe_products", "deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	stats.Prices, err = db.countTable(ctx, "np_stripe_prices", "deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	stats.Coupons, err = db.countTable(ctx, "np_stripe_coupons", "deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	stats.PromotionCodes, err = db.countTable(ctx, "np_stripe_promotion_codes", "")
	if err != nil {
		return nil, err
	}
	stats.Subscriptions, err = db.countTable(ctx, "np_stripe_subscriptions", "")
	if err != nil {
		return nil, err
	}
	stats.SubscriptionItems, err = db.countTable(ctx, "np_stripe_subscription_items", "")
	if err != nil {
		return nil, err
	}
	stats.Invoices, err = db.countTable(ctx, "np_stripe_invoices", "")
	if err != nil {
		return nil, err
	}
	stats.InvoiceItems, err = db.countTable(ctx, "np_stripe_invoice_items", "")
	if err != nil {
		return nil, err
	}
	stats.Charges, err = db.countTable(ctx, "np_stripe_charges", "")
	if err != nil {
		return nil, err
	}
	stats.Refunds, err = db.countTable(ctx, "np_stripe_refunds", "")
	if err != nil {
		return nil, err
	}
	stats.Disputes, err = db.countTable(ctx, "np_stripe_disputes", "")
	if err != nil {
		return nil, err
	}
	stats.PaymentIntents, err = db.countTable(ctx, "np_stripe_payment_intents", "")
	if err != nil {
		return nil, err
	}
	stats.SetupIntents, err = db.countTable(ctx, "np_stripe_setup_intents", "")
	if err != nil {
		return nil, err
	}
	stats.PaymentMethods, err = db.countTable(ctx, "np_stripe_payment_methods", "")
	if err != nil {
		return nil, err
	}
	stats.BalanceTransactions, err = db.countTable(ctx, "np_stripe_balance_transactions", "")
	if err != nil {
		return nil, err
	}
	stats.CheckoutSessions, err = db.countTable(ctx, "np_stripe_checkout_sessions", "")
	if err != nil {
		return nil, err
	}
	stats.TaxIDs, err = db.countTable(ctx, "np_stripe_tax_ids", "")
	if err != nil {
		return nil, err
	}
	stats.TaxRates, err = db.countTable(ctx, "np_stripe_tax_rates", "")
	if err != nil {
		return nil, err
	}

	// Last synced timestamp
	var lastSynced *time.Time
	err = db.Pool.QueryRow(ctx,
		"SELECT MAX(synced_at) FROM np_stripe_customers WHERE source_account_id = $1",
		db.SourceAccountID,
	).Scan(&lastSynced)
	if err != nil {
		return nil, err
	}
	if lastSynced != nil {
		s := lastSynced.Format(time.RFC3339)
		stats.LastSyncedAt = &s
	}

	return stats, nil
}

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

// ============================================================================
// Webhook event storage
// ============================================================================

// InsertWebhookEvent stores a raw webhook event in np_stripe_webhook_events.
func (db *DB) InsertWebhookEvent(ctx context.Context, event *StripeWebhookEvent) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_stripe_webhook_events (
			id, type, api_version, data, object_type, object_id,
			request_id, request_idempotency_key, livemode, pending_webhooks,
			processed, processed_at, error, retry_count,
			source_account_id, created_at, received_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO NOTHING`,
		event.ID, event.Type, nullStr(event.APIVersion), event.Data,
		nullStr(event.ObjectType), nullStr(event.ObjectID),
		nullStr(event.RequestID), nullStr(event.RequestIdempotencyKey),
		event.Livemode, event.PendingWebhooks,
		event.Processed, nullTime(event.ProcessedAt), nullStr(event.Error),
		event.RetryCount, db.SourceAccountID,
		nullTime(event.CreatedAt), nullTime(event.ReceivedAt),
	)
	return err
}

// MarkEventProcessed marks a webhook event as processed, optionally with an error.
func (db *DB) MarkEventProcessed(ctx context.Context, eventID string, errMsg string) error {
	if errMsg != "" {
		_, err := db.Pool.Exec(ctx,
			"UPDATE np_stripe_webhook_events SET processed = true, processed_at = NOW(), error = $2 WHERE id = $1",
			eventID, errMsg,
		)
		return err
	}
	_, err := db.Pool.Exec(ctx,
		"UPDATE np_stripe_webhook_events SET processed = true, processed_at = NOW() WHERE id = $1",
		eventID,
	)
	return err
}

// ============================================================================
// Upsert operations for webhook-driven data updates
// ============================================================================

// UpsertFromWebhookEvent stores or updates the object from a webhook event payload.
// The eventData is the raw JSON of event.data.object from Stripe.
func (db *DB) UpsertFromWebhookEvent(ctx context.Context, objectType string, eventData json.RawMessage) error {
	// Extract the id from the object
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(eventData, &obj); err != nil {
		return fmt.Errorf("failed to extract id from event data: %w", err)
	}
	if obj.ID == "" {
		return fmt.Errorf("event data has no id field")
	}

	table := objectTypeToTable(objectType)
	if table == "" {
		// Unknown object type, skip silently
		return nil
	}

	// Use a generic upsert: store the full JSON in the data column of webhook_events
	// For actual object tables, we do a lightweight upsert of synced_at to mark freshness.
	// The full sync operations handle detailed column-level upserts.
	_, err := db.Pool.Exec(ctx,
		fmt.Sprintf("UPDATE %s SET synced_at = NOW() WHERE id = $1 AND source_account_id = $2", table),
		obj.ID, db.SourceAccountID,
	)
	return err
}

// DeleteObject marks an object as deleted (soft delete) or hard-deletes it.
func (db *DB) DeleteObject(ctx context.Context, objectType string, objectID string) error {
	table := objectTypeToTable(objectType)
	if table == "" {
		return nil
	}
	// Tables with deleted_at use soft delete; others use hard delete
	switch table {
	case "np_stripe_customers", "np_stripe_products", "np_stripe_prices", "np_stripe_coupons":
		_, err := db.Pool.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND source_account_id = $2", table),
			objectID, db.SourceAccountID,
		)
		return err
	default:
		_, err := db.Pool.Exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND source_account_id = $2", table),
			objectID, db.SourceAccountID,
		)
		return err
	}
}

// ============================================================================
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

// ============================================================================
// Schema SQL
// ============================================================================

const schemaSQL = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Core Objects
CREATE TABLE IF NOT EXISTS np_stripe_customers (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	email VARCHAR(255),
	name VARCHAR(255),
	phone VARCHAR(50),
	description TEXT,
	currency VARCHAR(3),
	default_source VARCHAR(255),
	invoice_prefix VARCHAR(50),
	balance BIGINT DEFAULT 0,
	delinquent BOOLEAN DEFAULT FALSE,
	tax_exempt VARCHAR(20) DEFAULT 'none',
	metadata JSONB DEFAULT '{}',
	address JSONB,
	shipping JSONB,
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	deleted_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_customers_email ON np_stripe_customers(email);
CREATE INDEX IF NOT EXISTS idx_np_stripe_customers_created ON np_stripe_customers(created_at);
CREATE INDEX IF NOT EXISTS idx_np_stripe_customers_source ON np_stripe_customers(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_products (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	name VARCHAR(255) NOT NULL,
	description TEXT,
	active BOOLEAN DEFAULT TRUE,
	type VARCHAR(20) DEFAULT 'service',
	images JSONB DEFAULT '[]',
	metadata JSONB DEFAULT '{}',
	attributes JSONB DEFAULT '[]',
	shippable BOOLEAN,
	statement_descriptor VARCHAR(22),
	tax_code VARCHAR(255),
	unit_label VARCHAR(255),
	url VARCHAR(2048),
	default_price_id VARCHAR(255),
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	deleted_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_products_active ON np_stripe_products(active);
CREATE INDEX IF NOT EXISTS idx_np_stripe_products_source ON np_stripe_products(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_prices (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	product_id VARCHAR(255),
	active BOOLEAN DEFAULT TRUE,
	currency VARCHAR(3) NOT NULL,
	unit_amount BIGINT,
	unit_amount_decimal VARCHAR(50),
	type VARCHAR(20) NOT NULL,
	billing_scheme VARCHAR(20) DEFAULT 'per_unit',
	recurring JSONB,
	tiers JSONB,
	tiers_mode VARCHAR(20),
	transform_quantity JSONB,
	lookup_key VARCHAR(255),
	nickname VARCHAR(255),
	tax_behavior VARCHAR(20) DEFAULT 'unspecified',
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	deleted_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_prices_product ON np_stripe_prices(product_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_prices_active ON np_stripe_prices(active);
CREATE INDEX IF NOT EXISTS idx_np_stripe_prices_lookup ON np_stripe_prices(lookup_key);
CREATE INDEX IF NOT EXISTS idx_np_stripe_prices_source ON np_stripe_prices(source_account_id);

-- Discounts & Promotions
CREATE TABLE IF NOT EXISTS np_stripe_coupons (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	name VARCHAR(255),
	amount_off BIGINT,
	percent_off DECIMAL(5,2),
	currency VARCHAR(3),
	duration VARCHAR(20) NOT NULL,
	duration_in_months INTEGER,
	max_redemptions INTEGER,
	times_redeemed INTEGER DEFAULT 0,
	redeem_by TIMESTAMPTZ,
	valid BOOLEAN DEFAULT TRUE,
	applies_to JSONB,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	deleted_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_coupons_source ON np_stripe_coupons(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_discounts (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	coupon_id VARCHAR(255),
	customer_id VARCHAR(255),
	subscription_id VARCHAR(255),
	invoice_id VARCHAR(255),
	invoice_item_id VARCHAR(255),
	promotion_code_id VARCHAR(255),
	checkout_session_id VARCHAR(255),
	start_date TIMESTAMPTZ,
	end_date TIMESTAMPTZ,
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_discounts_source ON np_stripe_discounts(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_promotion_codes (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	coupon_id VARCHAR(255),
	code VARCHAR(255) NOT NULL,
	customer_id VARCHAR(255),
	active BOOLEAN DEFAULT TRUE,
	max_redemptions INTEGER,
	times_redeemed INTEGER DEFAULT 0,
	expires_at TIMESTAMPTZ,
	restrictions JSONB DEFAULT '{}',
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_promo_code ON np_stripe_promotion_codes(code);
CREATE INDEX IF NOT EXISTS idx_np_stripe_promo_coupon ON np_stripe_promotion_codes(coupon_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_promo_source ON np_stripe_promotion_codes(source_account_id);

-- Billing Objects
CREATE TABLE IF NOT EXISTS np_stripe_subscriptions (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	status VARCHAR(20) NOT NULL,
	current_period_start TIMESTAMPTZ,
	current_period_end TIMESTAMPTZ,
	cancel_at TIMESTAMPTZ,
	canceled_at TIMESTAMPTZ,
	cancel_at_period_end BOOLEAN DEFAULT FALSE,
	ended_at TIMESTAMPTZ,
	trial_start TIMESTAMPTZ,
	trial_end TIMESTAMPTZ,
	collection_method VARCHAR(20) DEFAULT 'charge_automatically',
	billing_cycle_anchor TIMESTAMPTZ,
	billing_thresholds JSONB,
	days_until_due INTEGER,
	default_payment_method_id VARCHAR(255),
	default_source VARCHAR(255),
	discount JSONB,
	items JSONB NOT NULL DEFAULT '[]',
	latest_invoice_id VARCHAR(255),
	pending_setup_intent VARCHAR(255),
	pending_update JSONB,
	schedule_id VARCHAR(255),
	start_date TIMESTAMPTZ,
	transfer_data JSONB,
	application_fee_percent DECIMAL(5,2),
	automatic_tax JSONB,
	payment_settings JSONB,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_subs_customer ON np_stripe_subscriptions(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_subs_status ON np_stripe_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_subs_period ON np_stripe_subscriptions(current_period_end);
CREATE INDEX IF NOT EXISTS idx_np_stripe_subs_source ON np_stripe_subscriptions(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_subscription_items (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	subscription_id VARCHAR(255),
	price_id VARCHAR(255),
	quantity INTEGER DEFAULT 1,
	billing_thresholds JSONB,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_sub_items_sub ON np_stripe_subscription_items(subscription_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_sub_items_source ON np_stripe_subscription_items(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_invoices (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	subscription_id VARCHAR(255),
	status VARCHAR(20),
	collection_method VARCHAR(20),
	currency VARCHAR(3) NOT NULL,
	amount_due BIGINT NOT NULL,
	amount_paid BIGINT DEFAULT 0,
	amount_remaining BIGINT DEFAULT 0,
	subtotal BIGINT NOT NULL,
	subtotal_excluding_tax BIGINT,
	total BIGINT NOT NULL,
	total_excluding_tax BIGINT,
	tax BIGINT,
	total_tax_amounts JSONB DEFAULT '[]',
	discount JSONB,
	discounts JSONB DEFAULT '[]',
	account_country VARCHAR(2),
	account_name VARCHAR(255),
	billing_reason VARCHAR(50),
	number VARCHAR(255),
	receipt_number VARCHAR(255),
	statement_descriptor VARCHAR(255),
	description TEXT,
	footer TEXT,
	customer_email VARCHAR(255),
	customer_name VARCHAR(255),
	customer_address JSONB,
	customer_phone VARCHAR(50),
	customer_shipping JSONB,
	customer_tax_exempt VARCHAR(20),
	customer_tax_ids JSONB DEFAULT '[]',
	default_payment_method_id VARCHAR(255),
	default_source VARCHAR(255),
	lines JSONB DEFAULT '[]',
	hosted_invoice_url TEXT,
	invoice_pdf TEXT,
	payment_intent_id VARCHAR(255),
	charge_id VARCHAR(255),
	attempt_count INTEGER DEFAULT 0,
	attempted BOOLEAN DEFAULT FALSE,
	auto_advance BOOLEAN DEFAULT TRUE,
	next_payment_attempt TIMESTAMPTZ,
	webhooks_delivered_at TIMESTAMPTZ,
	paid BOOLEAN DEFAULT FALSE,
	paid_out_of_band BOOLEAN DEFAULT FALSE,
	period_start TIMESTAMPTZ,
	period_end TIMESTAMPTZ,
	due_date TIMESTAMPTZ,
	effective_at TIMESTAMPTZ,
	finalized_at TIMESTAMPTZ,
	marked_uncollectible_at TIMESTAMPTZ,
	voided_at TIMESTAMPTZ,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_invoices_customer ON np_stripe_invoices(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_invoices_sub ON np_stripe_invoices(subscription_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_invoices_status ON np_stripe_invoices(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_invoices_created ON np_stripe_invoices(created_at);
CREATE INDEX IF NOT EXISTS idx_np_stripe_invoices_source ON np_stripe_invoices(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_invoice_items (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	invoice_id VARCHAR(255),
	subscription_id VARCHAR(255),
	subscription_item_id VARCHAR(255),
	price_id VARCHAR(255),
	amount BIGINT NOT NULL,
	currency VARCHAR(3) NOT NULL,
	description TEXT,
	discountable BOOLEAN DEFAULT TRUE,
	quantity INTEGER DEFAULT 1,
	unit_amount BIGINT,
	unit_amount_decimal VARCHAR(50),
	period_start TIMESTAMPTZ,
	period_end TIMESTAMPTZ,
	proration BOOLEAN DEFAULT FALSE,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_inv_items_inv ON np_stripe_invoice_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_inv_items_source ON np_stripe_invoice_items(source_account_id);

-- Payment Objects
CREATE TABLE IF NOT EXISTS np_stripe_payment_intents (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	invoice_id VARCHAR(255),
	amount BIGINT NOT NULL,
	amount_capturable BIGINT DEFAULT 0,
	amount_received BIGINT DEFAULT 0,
	currency VARCHAR(3) NOT NULL,
	status VARCHAR(30) NOT NULL,
	capture_method VARCHAR(20) DEFAULT 'automatic',
	confirmation_method VARCHAR(20) DEFAULT 'automatic',
	payment_method_id VARCHAR(255),
	payment_method_types JSONB DEFAULT '["card"]',
	setup_future_usage VARCHAR(20),
	client_secret VARCHAR(255),
	description TEXT,
	receipt_email VARCHAR(255),
	statement_descriptor VARCHAR(22),
	statement_descriptor_suffix VARCHAR(22),
	shipping JSONB,
	application_fee_amount BIGINT,
	transfer_data JSONB,
	transfer_group VARCHAR(255),
	on_behalf_of VARCHAR(255),
	cancellation_reason VARCHAR(50),
	canceled_at TIMESTAMPTZ,
	charges JSONB DEFAULT '[]',
	last_payment_error JSONB,
	next_action JSONB,
	processing JSONB,
	review VARCHAR(255),
	automatic_payment_methods JSONB,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pi_customer ON np_stripe_payment_intents(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pi_invoice ON np_stripe_payment_intents(invoice_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pi_status ON np_stripe_payment_intents(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pi_created ON np_stripe_payment_intents(created_at);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pi_source ON np_stripe_payment_intents(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_payment_methods (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	type VARCHAR(30) NOT NULL,
	billing_details JSONB,
	card JSONB,
	bank_account JSONB,
	sepa_debit JSONB,
	us_bank_account JSONB,
	link JSONB,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pm_customer ON np_stripe_payment_methods(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pm_type ON np_stripe_payment_methods(type);
CREATE INDEX IF NOT EXISTS idx_np_stripe_pm_source ON np_stripe_payment_methods(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_charges (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	payment_intent_id VARCHAR(255),
	invoice_id VARCHAR(255),
	amount BIGINT NOT NULL,
	amount_captured BIGINT DEFAULT 0,
	amount_refunded BIGINT DEFAULT 0,
	currency VARCHAR(3) NOT NULL,
	status VARCHAR(20) NOT NULL,
	paid BOOLEAN DEFAULT FALSE,
	captured BOOLEAN DEFAULT FALSE,
	refunded BOOLEAN DEFAULT FALSE,
	disputed BOOLEAN DEFAULT FALSE,
	failure_code VARCHAR(100),
	failure_message TEXT,
	outcome JSONB,
	description TEXT,
	receipt_email VARCHAR(255),
	receipt_number VARCHAR(255),
	receipt_url TEXT,
	statement_descriptor VARCHAR(22),
	statement_descriptor_suffix VARCHAR(22),
	payment_method_id VARCHAR(255),
	payment_method_details JSONB,
	billing_details JSONB,
	shipping JSONB,
	fraud_details JSONB,
	balance_transaction_id VARCHAR(255),
	application_fee_id VARCHAR(255),
	application_fee_amount BIGINT,
	transfer_id VARCHAR(255),
	transfer_group VARCHAR(255),
	on_behalf_of VARCHAR(255),
	source_transfer VARCHAR(255),
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_charges_customer ON np_stripe_charges(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_charges_pi ON np_stripe_charges(payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_charges_inv ON np_stripe_charges(invoice_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_charges_status ON np_stripe_charges(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_charges_created ON np_stripe_charges(created_at);
CREATE INDEX IF NOT EXISTS idx_np_stripe_charges_source ON np_stripe_charges(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_refunds (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	charge_id VARCHAR(255),
	payment_intent_id VARCHAR(255),
	amount BIGINT NOT NULL,
	currency VARCHAR(3) NOT NULL,
	status VARCHAR(20) NOT NULL,
	reason VARCHAR(50),
	receipt_number VARCHAR(255),
	description TEXT,
	failure_balance_transaction VARCHAR(255),
	failure_reason VARCHAR(100),
	balance_transaction_id VARCHAR(255),
	source_transfer_reversal VARCHAR(255),
	transfer_reversal VARCHAR(255),
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_refunds_charge ON np_stripe_refunds(charge_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_refunds_pi ON np_stripe_refunds(payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_refunds_source ON np_stripe_refunds(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_disputes (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	charge_id VARCHAR(255),
	payment_intent_id VARCHAR(255),
	amount BIGINT NOT NULL,
	currency VARCHAR(3) NOT NULL,
	status VARCHAR(30) NOT NULL,
	reason VARCHAR(50) NOT NULL,
	is_charge_refundable BOOLEAN DEFAULT FALSE,
	balance_transactions JSONB DEFAULT '[]',
	evidence JSONB DEFAULT '{}',
	evidence_details JSONB DEFAULT '{}',
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_disputes_charge ON np_stripe_disputes(charge_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_disputes_status ON np_stripe_disputes(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_disputes_source ON np_stripe_disputes(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_balance_transactions (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	amount BIGINT NOT NULL,
	currency VARCHAR(3) NOT NULL,
	net BIGINT NOT NULL,
	fee BIGINT DEFAULT 0,
	fee_details JSONB DEFAULT '[]',
	type VARCHAR(50) NOT NULL,
	status VARCHAR(20) NOT NULL,
	description TEXT,
	source VARCHAR(255),
	reporting_category VARCHAR(50),
	available_on TIMESTAMPTZ,
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_bal_type ON np_stripe_balance_transactions(type);
CREATE INDEX IF NOT EXISTS idx_np_stripe_bal_created ON np_stripe_balance_transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_np_stripe_bal_source ON np_stripe_balance_transactions(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_payouts (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	amount BIGINT NOT NULL,
	currency VARCHAR(3) NOT NULL,
	status VARCHAR(20) NOT NULL,
	type VARCHAR(20) NOT NULL,
	method VARCHAR(20),
	description TEXT,
	arrival_date TIMESTAMPTZ,
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_payouts_status ON np_stripe_payouts(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_payouts_source ON np_stripe_payouts(source_account_id);

-- Checkout Objects
CREATE TABLE IF NOT EXISTS np_stripe_checkout_sessions (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	customer_email VARCHAR(255),
	payment_intent_id VARCHAR(255),
	subscription_id VARCHAR(255),
	invoice_id VARCHAR(255),
	mode VARCHAR(20) NOT NULL,
	status VARCHAR(20),
	payment_status VARCHAR(20),
	currency VARCHAR(3),
	amount_total BIGINT,
	amount_subtotal BIGINT,
	total_details JSONB,
	success_url TEXT,
	cancel_url TEXT,
	url TEXT,
	client_reference_id VARCHAR(255),
	customer_creation VARCHAR(20),
	billing_address_collection VARCHAR(20),
	shipping_address_collection JSONB,
	shipping_cost JSONB,
	shipping_details JSONB,
	custom_text JSONB,
	consent JSONB,
	consent_collection JSONB,
	expires_at TIMESTAMPTZ,
	livemode BOOLEAN DEFAULT TRUE,
	locale VARCHAR(10),
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_checkout_customer ON np_stripe_checkout_sessions(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_checkout_status ON np_stripe_checkout_sessions(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_checkout_source ON np_stripe_checkout_sessions(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_setup_intents (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	payment_method_id VARCHAR(255),
	status VARCHAR(30) NOT NULL,
	usage VARCHAR(20) DEFAULT 'off_session',
	payment_method_types JSONB DEFAULT '["card"]',
	client_secret VARCHAR(255),
	description TEXT,
	cancellation_reason VARCHAR(50),
	last_setup_error JSONB,
	next_action JSONB,
	single_use_mandate VARCHAR(255),
	mandate VARCHAR(255),
	on_behalf_of VARCHAR(255),
	application VARCHAR(255),
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_si_customer ON np_stripe_setup_intents(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_si_status ON np_stripe_setup_intents(status);
CREATE INDEX IF NOT EXISTS idx_np_stripe_si_source ON np_stripe_setup_intents(source_account_id);

-- Tax Objects
CREATE TABLE IF NOT EXISTS np_stripe_tax_ids (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	customer_id VARCHAR(255),
	type VARCHAR(50) NOT NULL,
	value VARCHAR(255) NOT NULL,
	country VARCHAR(2),
	verification JSONB,
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_taxid_customer ON np_stripe_tax_ids(customer_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_taxid_source ON np_stripe_tax_ids(source_account_id);

CREATE TABLE IF NOT EXISTS np_stripe_tax_rates (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	display_name VARCHAR(255) NOT NULL,
	description TEXT,
	percentage DECIMAL(5,4) NOT NULL,
	inclusive BOOLEAN DEFAULT FALSE,
	active BOOLEAN DEFAULT TRUE,
	country VARCHAR(2),
	state VARCHAR(50),
	jurisdiction VARCHAR(255),
	tax_type VARCHAR(50),
	metadata JSONB DEFAULT '{}',
	created_at TIMESTAMPTZ,
	synced_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_taxrate_active ON np_stripe_tax_rates(active);
CREATE INDEX IF NOT EXISTS idx_np_stripe_taxrate_source ON np_stripe_tax_rates(source_account_id);

-- Webhook Events
CREATE TABLE IF NOT EXISTS np_stripe_webhook_events (
	id VARCHAR(255) NOT NULL,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	type VARCHAR(100) NOT NULL,
	api_version VARCHAR(50),
	data JSONB NOT NULL,
	object_type VARCHAR(100),
	object_id VARCHAR(255),
	request_id VARCHAR(255),
	request_idempotency_key VARCHAR(255),
	livemode BOOLEAN DEFAULT TRUE,
	pending_webhooks INTEGER DEFAULT 0,
	processed BOOLEAN DEFAULT FALSE,
	processed_at TIMESTAMPTZ,
	error TEXT,
	retry_count INTEGER DEFAULT 0,
	created_at TIMESTAMPTZ,
	received_at TIMESTAMPTZ DEFAULT NOW(),
	PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_stripe_events_type ON np_stripe_webhook_events(type);
CREATE INDEX IF NOT EXISTS idx_np_stripe_events_object ON np_stripe_webhook_events(object_type, object_id);
CREATE INDEX IF NOT EXISTS idx_np_stripe_events_processed ON np_stripe_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_np_stripe_events_created ON np_stripe_webhook_events(created_at);
CREATE INDEX IF NOT EXISTS idx_np_stripe_events_source ON np_stripe_webhook_events(source_account_id);

-- Analytics Views
CREATE OR REPLACE VIEW np_stripe_active_subscriptions AS
SELECT
	s.id AS subscription_id,
	s.source_account_id,
	s.status,
	c.id AS customer_id,
	c.email AS customer_email,
	c.name AS customer_name,
	s.current_period_start,
	s.current_period_end,
	s.cancel_at_period_end,
	s.items,
	s.metadata
FROM np_stripe_subscriptions s
LEFT JOIN np_stripe_customers c ON s.customer_id = c.id AND s.source_account_id = c.source_account_id
WHERE s.status IN ('active', 'trialing', 'past_due')
	AND (c.deleted_at IS NULL OR c.id IS NULL);

CREATE OR REPLACE VIEW np_stripe_daily_revenue AS
SELECT
	c.source_account_id,
	DATE(c.created_at) AS date,
	COUNT(*) AS charge_count,
	SUM(c.amount) AS gross_amount,
	SUM(c.amount_refunded) AS refunded_amount,
	SUM(c.amount - c.amount_refunded) AS net_amount,
	c.currency
FROM np_stripe_charges c
WHERE c.status = 'succeeded'
GROUP BY c.source_account_id, DATE(c.created_at), c.currency
ORDER BY date DESC;

CREATE OR REPLACE VIEW np_stripe_dispute_summary AS
SELECT
	d.source_account_id,
	d.status,
	d.reason,
	COUNT(*) AS dispute_count,
	SUM(d.amount) AS total_amount,
	d.currency
FROM np_stripe_disputes d
GROUP BY d.source_account_id, d.status, d.reason, d.currency
ORDER BY dispute_count DESC;

CREATE OR REPLACE VIEW np_stripe_failed_payments AS
SELECT
	pi.id AS payment_intent_id,
	pi.source_account_id,
	pi.amount,
	pi.currency,
	pi.status,
	pi.last_payment_error,
	c.id AS customer_id,
	c.email AS customer_email,
	c.name AS customer_name,
	pi.created_at
FROM np_stripe_payment_intents pi
LEFT JOIN np_stripe_customers c ON pi.customer_id = c.id AND pi.source_account_id = c.source_account_id
WHERE pi.status IN ('requires_payment_method', 'canceled')
	AND pi.last_payment_error IS NOT NULL
ORDER BY pi.created_at DESC;

CREATE OR REPLACE VIEW np_stripe_mrr AS
SELECT
	s.source_account_id,
	DATE_TRUNC('month', s.created_at) AS month,
	COUNT(*) AS subscription_count,
	SUM(
		CASE
			WHEN (s.items->0->'price'->'recurring'->>'interval') = 'month'
			THEN COALESCE((s.items->0->'price'->>'unit_amount')::BIGINT, 0)
			WHEN (s.items->0->'price'->'recurring'->>'interval') = 'year'
			THEN COALESCE((s.items->0->'price'->>'unit_amount')::BIGINT, 0) / 12
			ELSE 0
		END
	) AS mrr_cents
FROM np_stripe_subscriptions s
WHERE s.status IN ('active', 'trialing')
GROUP BY s.source_account_id, DATE_TRUNC('month', s.created_at)
ORDER BY month DESC;

CREATE OR REPLACE VIEW np_stripe_unified_payments AS
SELECT
	c.id AS payment_id,
	'charge' AS payment_type,
	c.source_account_id,
	c.customer_id,
	cust.email AS customer_email,
	cust.name AS customer_name,
	c.amount,
	c.amount_refunded,
	(c.amount - c.amount_refunded) AS net_amount,
	c.currency,
	c.status,
	c.description,
	c.invoice_id,
	c.payment_intent_id,
	c.payment_method_id,
	c.receipt_email,
	c.metadata,
	c.created_at
FROM np_stripe_charges c
LEFT JOIN np_stripe_customers cust ON c.customer_id = cust.id AND c.source_account_id = cust.source_account_id
WHERE c.status = 'succeeded'
ORDER BY c.created_at DESC;

CREATE OR REPLACE VIEW np_stripe_revenue_by_product AS
SELECT
	p.id AS product_id,
	p.name AS product_name,
	ch.source_account_id,
	COUNT(DISTINCT ch.id) AS charge_count,
	SUM(ch.amount) AS total_amount,
	ch.currency
FROM np_stripe_charges ch
JOIN np_stripe_invoices i ON ch.invoice_id = i.id AND ch.source_account_id = i.source_account_id
JOIN np_stripe_prices pr ON i.lines->0->>'price' = pr.id AND i.source_account_id = pr.source_account_id
JOIN np_stripe_products p ON pr.product_id = p.id AND pr.source_account_id = p.source_account_id
WHERE ch.status = 'succeeded'
GROUP BY p.id, p.name, ch.currency, ch.source_account_id
ORDER BY total_amount DESC;
`
