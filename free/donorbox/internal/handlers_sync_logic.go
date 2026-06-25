package internal

import (
	"context"
	"fmt"
	"log"
	"time"
)

// --- Sync logic --------------------------------------------------------------

// Size-cap exception: single-responsibility HTTP route handler — 56L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func runSync(ctx context.Context, db *DB, client *DonorboxClient) *SyncResult {
	started := time.Now()
	stats := SyncStats{}
	var errors []string

	type syncTask struct {
		name string
		fn   func() (int, error)
	}

	tasks := []syncTask{
		{"Campaigns", func() (int, error) { return syncCampaigns(ctx, db, client) }},
		{"Donors", func() (int, error) { return syncDonors(ctx, db, client) }},
		{"Donations", func() (int, error) { return syncDonations(ctx, db, client, "") }},
		{"Plans", func() (int, error) { return syncPlans(ctx, db, client) }},
		{"Events", func() (int, error) { return syncEvents(ctx, db, client) }},
		{"Tickets", func() (int, error) { return syncTickets(ctx, db, client) }},
	}

	for _, t := range tasks {
		log.Printf("[nself-donorbox] syncing %s...", t.name)
		count, err := t.fn()
		if err != nil {
			msg := fmt.Sprintf("%s: %v", t.name, err)
			errors = append(errors, msg)
			log.Printf("[nself-donorbox] sync error: %s", msg)
		} else {
			log.Printf("[nself-donorbox] %s: %d records", t.name, count)
		}

		switch t.name {
		case "Campaigns":
			stats.Campaigns = count
		case "Donors":
			stats.Donors = count
		case "Donations":
			stats.Donations = count
		case "Plans":
			stats.Plans = count
		case "Events":
			stats.Events = count
		case "Tickets":
			stats.Tickets = count
		}
	}

	now := time.Now()
	stats.LastSyncedAt = &now

	return &SyncResult{
		Success:  len(errors) == 0,
		Stats:    stats,
		Errors:   errors,
		Duration: time.Since(started).Milliseconds(),
	}
}

func runReconcile(ctx context.Context, db *DB, client *DonorboxClient, lookbackDays int) *SyncResult {
	started := time.Now()
	stats := SyncStats{}
	var errors []string

	since := time.Now().AddDate(0, 0, -lookbackDays)
	dateFrom := since.Format("2006-01-02")

	log.Printf("[nself-donorbox] reconciling last %d days (from %s)", lookbackDays, dateFrom)

	// Reconcile donations with date filter
	log.Println("[nself-donorbox] reconciling Donations...")
	count, err := syncDonations(ctx, db, client, dateFrom)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Donations: %v", err))
	}
	stats.Donations = count

	// Re-sync donors to pick up totals
	log.Println("[nself-donorbox] reconciling Donors...")
	count, err = syncDonors(ctx, db, client)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Donors: %v", err))
	}
	stats.Donors = count

	now := time.Now()
	stats.LastSyncedAt = &now

	return &SyncResult{
		Success:  len(errors) == 0,
		Stats:    stats,
		Errors:   errors,
		Duration: time.Since(started).Milliseconds(),
	}
}

