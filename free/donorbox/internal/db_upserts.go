package internal

import (
	"context"
	"encoding/json"
	"fmt"
)

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

