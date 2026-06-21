package internal

import (
	"context"
	"fmt"
	"log"
	"time"

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

