package internal

import (
	"context"
	"fmt"
	"time"
)


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

