package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate creates all 14 tables and their indexes if they do not exist.
// Size-cap exception: SQL DDL migration — 290L of linear SQL statements; splitting across files adds no value and breaks transactional migration semantics.
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

