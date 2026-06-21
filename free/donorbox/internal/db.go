package internal

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with donorbox-specific database operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB instance bound to the "primary" source account.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to the given source account ID.
func (db *DB) ForSourceAccount(accountID string) *DB {
	return &DB{pool: db.pool, sourceAccountID: accountID}
}

// Pool returns the underlying connection pool.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// InitSchema creates the 7 tables, indexes, and 5 views.
func (db *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("[nself-donorbox] initializing database schema")

	// 7 tables + indexes
	_, err := db.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_donorbox_campaigns (
			id INTEGER NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(255),
			slug VARCHAR(255),
			currency VARCHAR(10) DEFAULT 'USD',
			goal_amount NUMERIC(20, 2),
			total_raised NUMERIC(20, 2) DEFAULT 0,
			donations_count INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, source_account_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_campaigns_active ON np_donorbox_campaigns(is_active);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_campaigns_account ON np_donorbox_campaigns(source_account_id);

		CREATE TABLE IF NOT EXISTS np_donorbox_donors (
			id INTEGER NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			first_name VARCHAR(255),
			last_name VARCHAR(255),
			email VARCHAR(255),
			phone VARCHAR(50),
			address TEXT,
			city VARCHAR(255),
			state VARCHAR(100),
			zip_code VARCHAR(20),
			country VARCHAR(100),
			employer VARCHAR(255),
			donations_count INTEGER DEFAULT 0,
			last_donation_at TIMESTAMPTZ,
			total NUMERIC(20, 2) DEFAULT 0,
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, source_account_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donors_email ON np_donorbox_donors(email);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donors_account ON np_donorbox_donors(source_account_id);

		CREATE TABLE IF NOT EXISTS np_donorbox_donations (
			id INTEGER NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			campaign_id INTEGER,
			campaign_name VARCHAR(255),
			donor_id INTEGER,
			donor_email VARCHAR(255),
			donor_name VARCHAR(255),
			amount NUMERIC(20, 2) DEFAULT 0,
			converted_amount NUMERIC(20, 2),
			converted_net_amount NUMERIC(20, 2),
			amount_refunded NUMERIC(20, 2) DEFAULT 0,
			currency VARCHAR(10) DEFAULT 'USD',
			donation_type VARCHAR(50),
			donation_date TIMESTAMPTZ,
			processing_fee NUMERIC(20, 2),
			status VARCHAR(50),
			recurring BOOLEAN DEFAULT false,
			comment TEXT,
			designation VARCHAR(255),
			stripe_charge_id VARCHAR(255),
			paypal_transaction_id VARCHAR(255),
			questions JSONB DEFAULT '[]',
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, source_account_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_campaign ON np_donorbox_donations(campaign_id);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_donor ON np_donorbox_donations(donor_id);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_date ON np_donorbox_donations(donation_date DESC);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_status ON np_donorbox_donations(status);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_stripe ON np_donorbox_donations(stripe_charge_id);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_paypal ON np_donorbox_donations(paypal_transaction_id);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_account ON np_donorbox_donations(source_account_id);

		CREATE TABLE IF NOT EXISTS np_donorbox_plans (
			id INTEGER NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			campaign_id INTEGER,
			campaign_name VARCHAR(255),
			donor_id INTEGER,
			donor_email VARCHAR(255),
			type VARCHAR(50),
			amount NUMERIC(20, 2) DEFAULT 0,
			currency VARCHAR(10) DEFAULT 'USD',
			status VARCHAR(50),
			started_at TIMESTAMPTZ,
			last_donation_date TIMESTAMPTZ,
			next_donation_date TIMESTAMPTZ,
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, source_account_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_plans_status ON np_donorbox_plans(status);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_plans_donor ON np_donorbox_plans(donor_id);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_plans_account ON np_donorbox_plans(source_account_id);

		CREATE TABLE IF NOT EXISTS np_donorbox_events (
			id INTEGER NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(255),
			slug VARCHAR(255),
			description TEXT,
			start_date TIMESTAMPTZ,
			end_date TIMESTAMPTZ,
			timezone VARCHAR(50),
			venue_name VARCHAR(255),
			address TEXT,
			city VARCHAR(255),
			state VARCHAR(100),
			country VARCHAR(100),
			zip_code VARCHAR(20),
			currency VARCHAR(10) DEFAULT 'USD',
			tickets_count INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, source_account_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_events_active ON np_donorbox_events(is_active);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_events_account ON np_donorbox_events(source_account_id);

		CREATE TABLE IF NOT EXISTS np_donorbox_tickets (
			id INTEGER NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			event_id INTEGER,
			event_name VARCHAR(255),
			donor_id INTEGER,
			donor_email VARCHAR(255),
			ticket_type VARCHAR(100),
			quantity INTEGER DEFAULT 0,
			amount NUMERIC(20, 2) DEFAULT 0,
			currency VARCHAR(10) DEFAULT 'USD',
			status VARCHAR(50),
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, source_account_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_tickets_event ON np_donorbox_tickets(event_id);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_tickets_account ON np_donorbox_tickets(source_account_id);

		CREATE TABLE IF NOT EXISTS np_donorbox_webhook_events (
			id VARCHAR(255) PRIMARY KEY,
			event_type VARCHAR(255),
			payload JSONB DEFAULT '{}',
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			processed BOOLEAN DEFAULT false,
			processed_at TIMESTAMPTZ,
			error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_donorbox_webhook_events_type ON np_donorbox_webhook_events(event_type);
		CREATE INDEX IF NOT EXISTS idx_np_donorbox_webhook_events_account ON np_donorbox_webhook_events(source_account_id);
	`)
	if err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	log.Println("[nself-donorbox] created 7 tables")

	// 5 views (use np_donorbox_ prefix)
	_, err = db.pool.Exec(ctx, `
		CREATE OR REPLACE VIEW np_donorbox_unified_donations AS
		SELECT
			d.id AS donation_id,
			d.source_account_id,
			d.campaign_id,
			d.campaign_name,
			d.donor_id,
			d.donor_email,
			d.donor_name,
			d.amount,
			d.amount_refunded,
			(d.amount - d.amount_refunded) AS net_amount,
			d.currency,
			d.donation_type,
			d.donation_date,
			d.status,
			d.recurring,
			d.comment,
			d.designation,
			d.stripe_charge_id,
			d.paypal_transaction_id,
			d.processing_fee
		FROM np_donorbox_donations d
		WHERE d.status != 'refunded'
		ORDER BY d.donation_date DESC;

		CREATE OR REPLACE VIEW np_donorbox_campaign_summary AS
		SELECT
			c.id AS campaign_id,
			c.name,
			c.source_account_id,
			c.goal_amount,
			c.total_raised,
			c.donations_count,
			c.is_active,
			c.currency,
			COUNT(DISTINCT d.donor_id) AS unique_donors
		FROM np_donorbox_campaigns c
		LEFT JOIN np_donorbox_donations d ON d.campaign_id = c.id AND d.source_account_id = c.source_account_id
		GROUP BY c.id, c.name, c.source_account_id, c.goal_amount, c.total_raised, c.donations_count, c.is_active, c.currency;

		CREATE OR REPLACE VIEW np_donorbox_daily_donations AS
		SELECT
			d.source_account_id,
			DATE(d.donation_date) AS donation_day,
			d.currency,
			COUNT(*) AS donation_count,
			SUM(d.amount) AS total_amount,
			SUM(d.amount - d.amount_refunded) AS net_amount
		FROM np_donorbox_donations d
		WHERE d.donation_date IS NOT NULL
		GROUP BY d.source_account_id, DATE(d.donation_date), d.currency
		ORDER BY donation_day DESC;

		CREATE OR REPLACE VIEW np_donorbox_recurring_summary AS
		SELECT
			p.source_account_id,
			p.status,
			p.currency,
			COUNT(*) AS plan_count,
			SUM(p.amount) AS total_recurring_amount
		FROM np_donorbox_plans p
		GROUP BY p.source_account_id, p.status, p.currency;

		CREATE OR REPLACE VIEW np_donorbox_top_donors AS
		SELECT
			d.id AS donor_id,
			d.source_account_id,
			d.email,
			CONCAT(d.first_name, ' ', d.last_name) AS name,
			d.total,
			d.donations_count,
			d.last_donation_at
		FROM np_donorbox_donors d
		WHERE d.total > 0
		ORDER BY d.total DESC;
	`)
	if err != nil {
		return fmt.Errorf("create views: %w", err)
	}

	log.Println("[nself-donorbox] created 5 views")
	log.Println("[nself-donorbox] schema initialized")
	return nil
}

