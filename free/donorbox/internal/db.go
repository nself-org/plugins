package internal

import (
	"context"
	"encoding/json"
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

// --- Upsert methods ----------------------------------------------------------

// UpsertCampaigns inserts or updates campaign records.
func (db *DB) UpsertCampaigns(ctx context.Context, records []Campaign) (int, error) {
	count := 0
	for _, r := range records {
		_, err := db.pool.Exec(ctx, `
			INSERT INTO np_donorbox_campaigns
				(id, source_account_id, name, slug, currency, goal_amount, total_raised, donations_count, is_active, created_at, updated_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
			ON CONFLICT (id, source_account_id) DO UPDATE SET
				name = EXCLUDED.name, slug = EXCLUDED.slug, goal_amount = EXCLUDED.goal_amount,
				total_raised = EXCLUDED.total_raised, donations_count = EXCLUDED.donations_count,
				is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at, synced_at = NOW()
		`, r.ID, db.sourceAccountID, r.Name, r.Slug, r.Currency, r.GoalAmount,
			r.TotalRaised, r.DonationsCount, r.IsActive, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return count, fmt.Errorf("upsert campaign %d: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

// UpsertDonors inserts or updates donor records.
func (db *DB) UpsertDonors(ctx context.Context, records []Donor) (int, error) {
	count := 0
	for _, r := range records {
		_, err := db.pool.Exec(ctx, `
			INSERT INTO np_donorbox_donors
				(id, source_account_id, first_name, last_name, email, phone, address, city, state, zip_code, country, employer, donations_count, last_donation_at, total, created_at, updated_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
			ON CONFLICT (id, source_account_id) DO UPDATE SET
				first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, email = EXCLUDED.email,
				phone = EXCLUDED.phone, address = EXCLUDED.address, city = EXCLUDED.city,
				state = EXCLUDED.state, zip_code = EXCLUDED.zip_code, country = EXCLUDED.country,
				employer = EXCLUDED.employer, donations_count = EXCLUDED.donations_count,
				last_donation_at = EXCLUDED.last_donation_at, total = EXCLUDED.total,
				updated_at = EXCLUDED.updated_at, synced_at = NOW()
		`, r.ID, db.sourceAccountID, r.FirstName, r.LastName, r.Email, r.Phone,
			r.Address, r.City, r.State, r.ZipCode, r.Country, r.Employer,
			r.DonationsCount, r.LastDonationAt, r.Total, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return count, fmt.Errorf("upsert donor %d: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

// UpsertDonations inserts or updates donation records.
func (db *DB) UpsertDonations(ctx context.Context, records []Donation) (int, error) {
	count := 0
	for _, r := range records {
		q := r.Questions
		if q == nil {
			q = json.RawMessage("[]")
		}
		_, err := db.pool.Exec(ctx, `
			INSERT INTO np_donorbox_donations
				(id, source_account_id, campaign_id, campaign_name, donor_id, donor_email, donor_name,
				 amount, converted_amount, converted_net_amount, amount_refunded, currency, donation_type,
				 donation_date, processing_fee, status, recurring, comment, designation,
				 stripe_charge_id, paypal_transaction_id, questions, created_at, updated_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW())
			ON CONFLICT (id, source_account_id) DO UPDATE SET
				status = EXCLUDED.status, amount_refunded = EXCLUDED.amount_refunded,
				converted_amount = EXCLUDED.converted_amount, converted_net_amount = EXCLUDED.converted_net_amount,
				comment = EXCLUDED.comment, updated_at = EXCLUDED.updated_at, synced_at = NOW()
		`, r.ID, db.sourceAccountID, r.CampaignID, r.CampaignName, r.DonorID, r.DonorEmail, r.DonorName,
			r.Amount, r.ConvertedAmount, r.ConvertedNetAmount, r.AmountRefunded, r.Currency, r.DonationType,
			r.DonationDate, r.ProcessingFee, r.Status, r.Recurring, r.Comment, r.Designation,
			r.StripeChargeID, r.PaypalTxnID, q, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return count, fmt.Errorf("upsert donation %d: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

// UpsertPlans inserts or updates recurring plan records.
func (db *DB) UpsertPlans(ctx context.Context, records []Plan) (int, error) {
	count := 0
	for _, r := range records {
		_, err := db.pool.Exec(ctx, `
			INSERT INTO np_donorbox_plans
				(id, source_account_id, campaign_id, campaign_name, donor_id, donor_email, type, amount, currency, status, started_at, last_donation_date, next_donation_date, created_at, updated_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
			ON CONFLICT (id, source_account_id) DO UPDATE SET
				status = EXCLUDED.status, amount = EXCLUDED.amount, last_donation_date = EXCLUDED.last_donation_date,
				next_donation_date = EXCLUDED.next_donation_date, updated_at = EXCLUDED.updated_at, synced_at = NOW()
		`, r.ID, db.sourceAccountID, r.CampaignID, r.CampaignName, r.DonorID, r.DonorEmail,
			r.Type, r.Amount, r.Currency, r.Status, r.StartedAt,
			r.LastDonationDate, r.NextDonationDate, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return count, fmt.Errorf("upsert plan %d: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

// UpsertEvents inserts or updates event records.
func (db *DB) UpsertEvents(ctx context.Context, records []Event) (int, error) {
	count := 0
	for _, r := range records {
		_, err := db.pool.Exec(ctx, `
			INSERT INTO np_donorbox_events
				(id, source_account_id, name, slug, description, start_date, end_date, timezone, venue_name, address, city, state, country, zip_code, currency, tickets_count, is_active, created_at, updated_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
			ON CONFLICT (id, source_account_id) DO UPDATE SET
				name = EXCLUDED.name, description = EXCLUDED.description, tickets_count = EXCLUDED.tickets_count,
				is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at, synced_at = NOW()
		`, r.ID, db.sourceAccountID, r.Name, r.Slug, r.Description, r.StartDate, r.EndDate,
			r.Timezone, r.VenueName, r.Address, r.City, r.State, r.Country, r.ZipCode,
			r.Currency, r.TicketsCount, r.IsActive, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return count, fmt.Errorf("upsert event %d: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

// UpsertTickets inserts or updates ticket records.
func (db *DB) UpsertTickets(ctx context.Context, records []Ticket) (int, error) {
	count := 0
	for _, r := range records {
		_, err := db.pool.Exec(ctx, `
			INSERT INTO np_donorbox_tickets
				(id, source_account_id, event_id, event_name, donor_id, donor_email, ticket_type, quantity, amount, currency, status, created_at, updated_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
			ON CONFLICT (id, source_account_id) DO UPDATE SET
				status = EXCLUDED.status, quantity = EXCLUDED.quantity, amount = EXCLUDED.amount,
				updated_at = EXCLUDED.updated_at, synced_at = NOW()
		`, r.ID, db.sourceAccountID, r.EventID, r.EventName, r.DonorID, r.DonorEmail,
			r.TicketType, r.Quantity, r.Amount, r.Currency, r.Status, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return count, fmt.Errorf("upsert ticket %d: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

// --- Webhook event storage ---------------------------------------------------

// InsertWebhookEvent stores a raw webhook event.
func (db *DB) InsertWebhookEvent(ctx context.Context, id, eventType string, payload json.RawMessage) error {
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	_, err := db.pool.Exec(ctx, `
		INSERT INTO np_donorbox_webhook_events (id, event_type, payload, source_account_id, created_at, synced_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, id, eventType, payload, db.sourceAccountID)
	return err
}

// MarkEventProcessed marks a webhook event as processed, optionally with an error.
func (db *DB) MarkEventProcessed(ctx context.Context, eventID string, errMsg *string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE np_donorbox_webhook_events SET processed = true, processed_at = NOW(), error = $2
		WHERE id = $1 AND source_account_id = $3
	`, eventID, errMsg, db.sourceAccountID)
	return err
}

// --- Statistics --------------------------------------------------------------

// GetStats returns counts per entity for the current source account.
func (db *DB) GetStats(ctx context.Context) (*SyncStats, error) {
	stats := &SyncStats{}

	tables := []struct {
		field *int
		table string
	}{
		{&stats.Campaigns, "np_donorbox_campaigns"},
		{&stats.Donors, "np_donorbox_donors"},
		{&stats.Donations, "np_donorbox_donations"},
		{&stats.Plans, "np_donorbox_plans"},
		{&stats.Events, "np_donorbox_events"},
		{&stats.Tickets, "np_donorbox_tickets"},
	}

	for _, t := range tables {
		err := db.pool.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE source_account_id = $1", t.table),
			db.sourceAccountID,
		).Scan(t.field)
		if err != nil {
			// Table may not exist yet; continue
			*t.field = 0
		}
	}

	var lastSync *time.Time
	err := db.pool.QueryRow(ctx,
		"SELECT MAX(synced_at) FROM np_donorbox_donations WHERE source_account_id = $1",
		db.sourceAccountID,
	).Scan(&lastSync)
	if err == nil {
		stats.LastSyncedAt = lastSync
	}

	return stats, nil
}

// --- Query methods -----------------------------------------------------------

// QueryCampaigns returns campaigns for the current source account.
func (db *DB) QueryCampaigns(ctx context.Context, limit, offset int) ([]Campaign, error) {
	query := "SELECT id, source_account_id, name, slug, currency, goal_amount, total_raised, donations_count, is_active, created_at, updated_at, synced_at FROM np_donorbox_campaigns WHERE source_account_id = $1 ORDER BY created_at DESC"
	args := []interface{}{db.sourceAccountID}
	idx := 2

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.Name, &c.Slug, &c.Currency,
			&c.GoalAmount, &c.TotalRaised, &c.DonationsCount, &c.IsActive,
			&c.CreatedAt, &c.UpdatedAt, &c.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

// QueryDonors returns donors for the current source account.
func (db *DB) QueryDonors(ctx context.Context, limit, offset int) ([]Donor, error) {
	query := "SELECT id, source_account_id, first_name, last_name, email, phone, address, city, state, zip_code, country, employer, donations_count, last_donation_at, total, created_at, updated_at, synced_at FROM np_donorbox_donors WHERE source_account_id = $1 ORDER BY total DESC"
	args := []interface{}{db.sourceAccountID}
	idx := 2

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Donor
	for rows.Next() {
		var d Donor
		if err := rows.Scan(&d.ID, &d.SourceAccountID, &d.FirstName, &d.LastName, &d.Email,
			&d.Phone, &d.Address, &d.City, &d.State, &d.ZipCode, &d.Country, &d.Employer,
			&d.DonationsCount, &d.LastDonationAt, &d.Total, &d.CreatedAt, &d.UpdatedAt, &d.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// QueryDonations returns donations for the current source account with optional status filter.
func (db *DB) QueryDonations(ctx context.Context, status string, limit, offset int) ([]Donation, error) {
	query := "SELECT id, source_account_id, campaign_id, campaign_name, donor_id, donor_email, donor_name, amount, converted_amount, converted_net_amount, amount_refunded, currency, donation_type, donation_date, processing_fee, status, recurring, comment, designation, stripe_charge_id, paypal_transaction_id, questions, created_at, updated_at, synced_at FROM np_donorbox_donations WHERE source_account_id = $1"
	args := []interface{}{db.sourceAccountID}
	idx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}

	query += " ORDER BY donation_date DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Donation
	for rows.Next() {
		var d Donation
		if err := rows.Scan(&d.ID, &d.SourceAccountID, &d.CampaignID, &d.CampaignName,
			&d.DonorID, &d.DonorEmail, &d.DonorName, &d.Amount, &d.ConvertedAmount,
			&d.ConvertedNetAmount, &d.AmountRefunded, &d.Currency, &d.DonationType,
			&d.DonationDate, &d.ProcessingFee, &d.Status, &d.Recurring, &d.Comment,
			&d.Designation, &d.StripeChargeID, &d.PaypalTxnID, &d.Questions,
			&d.CreatedAt, &d.UpdatedAt, &d.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// QueryPlans returns recurring plans for the current source account with optional status filter.
func (db *DB) QueryPlans(ctx context.Context, status string, limit, offset int) ([]Plan, error) {
	query := "SELECT id, source_account_id, campaign_id, campaign_name, donor_id, donor_email, type, amount, currency, status, started_at, last_donation_date, next_donation_date, created_at, updated_at, synced_at FROM np_donorbox_plans WHERE source_account_id = $1"
	args := []interface{}{db.sourceAccountID}
	idx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.CampaignID, &p.CampaignName,
			&p.DonorID, &p.DonorEmail, &p.Type, &p.Amount, &p.Currency, &p.Status,
			&p.StartedAt, &p.LastDonationDate, &p.NextDonationDate,
			&p.CreatedAt, &p.UpdatedAt, &p.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// QueryWebhookEvents returns recent webhook events.
func (db *DB) QueryWebhookEvents(ctx context.Context, limit int) ([]WebhookEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.pool.Query(ctx, `
		SELECT id, event_type, payload, source_account_id, processed, processed_at, error, created_at, synced_at
		FROM np_donorbox_webhook_events WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2
	`, db.sourceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload, &e.SourceAccountID,
			&e.Processed, &e.ProcessedAt, &e.Error, &e.CreatedAt, &e.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}
