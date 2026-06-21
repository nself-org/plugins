package internal

import (
	"context"
	"fmt"
)

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

