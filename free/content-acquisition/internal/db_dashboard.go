package internal

import (
	"context"
)

// =========================================================================
// Dashboard Summary
// =========================================================================

// GetDashboardSummary returns aggregate counts for the dashboard.
func (d *DB) GetDashboardSummary(accountID string) (*DashboardSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var s DashboardSummary

	// Downloads stats
	err := d.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE state NOT IN ('completed', 'failed', 'cancelled'))::int,
		   COUNT(*) FILTER (WHERE state = 'completed' AND updated_at >= CURRENT_DATE)::int,
		   COUNT(*) FILTER (WHERE state = 'failed' AND updated_at >= CURRENT_DATE)::int
		 FROM np_contentacquisition_downloads WHERE source_account_id = $1`,
		accountID,
	).Scan(&s.ActiveDownloads, &s.CompletedToday, &s.FailedToday)
	if err != nil {
		return nil, err
	}

	// Active subscriptions
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_acquisition_subscriptions
		 WHERE source_account_id = $1 AND enabled = true`, accountID,
	).Scan(&s.ActiveSubscriptions)
	if err != nil {
		return nil, err
	}

	// Monitored movies
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_movie_monitoring
		 WHERE source_account_id = $1 AND status != 'downloaded'`, accountID,
	).Scan(&s.MonitoredMovies)
	if err != nil {
		return nil, err
	}

	// Enabled feeds
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_rss_feeds
		 WHERE source_account_id = $1 AND enabled = true`, accountID,
	).Scan(&s.EnabledFeeds)
	if err != nil {
		return nil, err
	}

	// Enabled rules
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_download_rules
		 WHERE source_account_id = $1 AND enabled = true`, accountID,
	).Scan(&s.EnabledRules)
	if err != nil {
		return nil, err
	}

	// Queue depth
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int
		 FROM np_contentacquisition_download_queue q
		 JOIN np_contentacquisition_downloads d ON d.id = q.download_id
		 WHERE d.source_account_id = $1`, accountID,
	).Scan(&s.QueueDepth)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

