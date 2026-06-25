package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with a source account identifier.
// All queries are scoped to the SourceAccountID where applicable.
type DB struct {
	Pool            *pgxpool.Pool
	SourceAccountID string
}

// NewDB creates a new DB instance.
func NewDB(pool *pgxpool.Pool, sourceAccountID string) *DB {
	return &DB{
		Pool:            pool,
		SourceAccountID: sourceAccountID,
	}
}

// Migrate creates all required tables and indexes if they do not exist.
// Size-cap exception: SQL DDL migration — 209L of linear SQL statements; splitting across files adds no value and breaks transactional migration semantics.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_shopify_shops (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			name              TEXT NOT NULL DEFAULT '',
			email             TEXT,
			domain            TEXT,
			myshopify_domain  TEXT NOT NULL DEFAULT '',
			country           TEXT,
			currency          TEXT NOT NULL DEFAULT 'USD',
			timezone          TEXT,
			plan_name         TEXT,
			plan_display_name TEXT,
			money_format      TEXT,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_shops_shopify_id_account
			ON np_shopify_shops (shopify_id, source_account_id);

		CREATE TABLE IF NOT EXISTS np_shopify_products (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			title             TEXT NOT NULL DEFAULT '',
			body_html         TEXT,
			vendor            TEXT,
			product_type      TEXT,
			handle            TEXT,
			status            TEXT NOT NULL DEFAULT 'active',
			tags              TEXT,
			images            JSONB NOT NULL DEFAULT '[]',
			options           JSONB NOT NULL DEFAULT '[]',
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			published_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_products_shopify_id_account
			ON np_shopify_products (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_products_status
			ON np_shopify_products (status);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_products_vendor
			ON np_shopify_products (vendor);

		CREATE TABLE IF NOT EXISTS np_shopify_variants (
			id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id         BIGINT NOT NULL,
			product_id         TEXT NOT NULL,
			title              TEXT,
			price              TEXT,
			compare_at_price   TEXT,
			sku                TEXT,
			barcode            TEXT,
			position           INT NOT NULL DEFAULT 1,
			inventory_quantity INT NOT NULL DEFAULT 0,
			inventory_item_id  BIGINT,
			weight             DOUBLE PRECISION,
			weight_unit        TEXT,
			option1            TEXT,
			option2            TEXT,
			option3            TEXT,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id  TEXT NOT NULL DEFAULT 'primary',
			synced_at          TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_variants_shopify_id_account
			ON np_shopify_variants (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_variants_product_id
			ON np_shopify_variants (product_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_variants_sku
			ON np_shopify_variants (sku);

		CREATE TABLE IF NOT EXISTS np_shopify_collections (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			title             TEXT NOT NULL DEFAULT '',
			body_html         TEXT,
			handle            TEXT,
			sort_order        TEXT,
			collection_type   TEXT,
			image             JSONB,
			published_at      TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_collections_shopify_id_account
			ON np_shopify_collections (shopify_id, source_account_id);

		CREATE TABLE IF NOT EXISTS np_shopify_customers (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			email             TEXT,
			first_name        TEXT,
			last_name         TEXT,
			phone             TEXT,
			orders_count      INT NOT NULL DEFAULT 0,
			total_spent       TEXT,
			currency          TEXT,
			tags              TEXT,
			addresses         JSONB NOT NULL DEFAULT '[]',
			default_address   JSONB,
			accepts_marketing BOOLEAN NOT NULL DEFAULT false,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_customers_shopify_id_account
			ON np_shopify_customers (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_customers_email
			ON np_shopify_customers (email);

		CREATE TABLE IF NOT EXISTS np_shopify_orders (
			id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id         BIGINT NOT NULL,
			name               TEXT NOT NULL DEFAULT '',
			email              TEXT,
			total_price        TEXT,
			subtotal_price     TEXT,
			total_tax          TEXT,
			total_discounts    TEXT,
			currency           TEXT NOT NULL DEFAULT 'USD',
			financial_status   TEXT,
			fulfillment_status TEXT,
			customer_id        BIGINT,
			line_items         JSONB NOT NULL DEFAULT '[]',
			shipping_address   JSONB,
			billing_address    JSONB,
			note               TEXT,
			tags               TEXT,
			gateway            TEXT,
			confirmed          BOOLEAN NOT NULL DEFAULT false,
			cancelled_at       TIMESTAMPTZ,
			cancel_reason      TEXT,
			closed_at          TIMESTAMPTZ,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			processed_at       TIMESTAMPTZ,
			source_account_id  TEXT NOT NULL DEFAULT 'primary',
			synced_at          TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_orders_shopify_id_account
			ON np_shopify_orders (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_orders_financial_status
			ON np_shopify_orders (financial_status);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_orders_fulfillment_status
			ON np_shopify_orders (fulfillment_status);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_orders_customer_id
			ON np_shopify_orders (customer_id);

		CREATE TABLE IF NOT EXISTS np_shopify_order_items (
			id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id         BIGINT NOT NULL,
			order_id           TEXT NOT NULL,
			product_id         BIGINT,
			variant_id         BIGINT,
			title              TEXT NOT NULL DEFAULT '',
			quantity           INT NOT NULL DEFAULT 0,
			price              TEXT,
			sku                TEXT,
			vendor             TEXT,
			fulfillment_status TEXT,
			source_account_id  TEXT NOT NULL DEFAULT 'primary',
			synced_at          TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_order_items_shopify_id_account
			ON np_shopify_order_items (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_order_items_order_id
			ON np_shopify_order_items (order_id);

		CREATE TABLE IF NOT EXISTS np_shopify_inventory (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			inventory_item_id BIGINT NOT NULL,
			location_id       BIGINT NOT NULL,
			available         INT NOT NULL DEFAULT 0,
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_inventory_item_location_account
			ON np_shopify_inventory (inventory_item_id, location_id, source_account_id);

		CREATE TABLE IF NOT EXISTS np_shopify_webhook_events (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_event_id  TEXT,
			topic             TEXT NOT NULL,
			shop_domain       TEXT,
			body              JSONB NOT NULL DEFAULT '{}',
			processed         BOOLEAN NOT NULL DEFAULT false,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary'
		);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_webhook_events_topic
			ON np_shopify_webhook_events (topic);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_webhook_events_processed
			ON np_shopify_webhook_events (processed);
	`)
	return err
}

