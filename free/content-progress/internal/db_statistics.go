package internal

import (
	"context"
	"fmt"
	"time"
)

// =========================================================================
// Statistics
// =========================================================================

// GetUserStats returns aggregated statistics for a single user.
func (d *DB) GetUserStats(userID string) (*UserStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var totalSeconds float64
	var completed, inProgress, watchlistCount, favoritesCount int64
	var mostWatchedType *string
	var recentActivity *time.Time

	err := d.pool.QueryRow(ctx,
		`WITH watch_time AS (
			SELECT COALESCE(SUM(position_seconds), 0) as total_seconds
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
		),
		counts AS (
			SELECT
				COUNT(*) FILTER (WHERE completed = TRUE) as completed,
				COUNT(*) FILTER (WHERE completed = FALSE AND progress_percent > 1) as in_progress
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
		),
		watchlist AS (
			SELECT COUNT(*) as count
			FROM np_progress_watchlists
			WHERE source_account_id = $1 AND user_id = $2
		),
		favorites AS (
			SELECT COUNT(*) as count
			FROM np_progress_favorites
			WHERE source_account_id = $1 AND user_id = $2
		),
		most_watched AS (
			SELECT content_type
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
			GROUP BY content_type
			ORDER BY COUNT(*) DESC
			LIMIT 1
		),
		recent AS (
			SELECT MAX(updated_at) as last_activity
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
		)
		SELECT
			w.total_seconds,
			c.completed,
			c.in_progress,
			wl.count,
			f.count,
			mw.content_type,
			r.last_activity
		FROM watch_time w
		CROSS JOIN counts c
		CROSS JOIN watchlist wl
		CROSS JOIN favorites f
		LEFT JOIN most_watched mw ON TRUE
		LEFT JOIN recent r ON TRUE`,
		d.sourceAccountID, userID,
	).Scan(&totalSeconds, &completed, &inProgress, &watchlistCount, &favoritesCount, &mostWatchedType, &recentActivity)
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
	}

	return &UserStats{
		TotalWatchTimeSeconds: totalSeconds,
		TotalWatchTimeHours:   totalSeconds / 3600,
		ContentCompleted:      completed,
		ContentInProgress:     inProgress,
		WatchlistCount:        watchlistCount,
		FavoritesCount:        favoritesCount,
		MostWatchedType:       mostWatchedType,
		RecentActivity:        recentActivity,
	}, nil
}

// GetPluginStats returns aggregated plugin-wide statistics.
func (d *DB) GetPluginStats() (*PluginStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var totalUsers, totalPositions, totalCompleted, totalInProgress int64
	var totalWatchlist, totalFavorites, totalHistoryEvts int64
	var lastActivity *time.Time

	err := d.pool.QueryRow(ctx,
		`WITH users AS (
			SELECT COUNT(DISTINCT user_id) as count
			FROM np_progress_positions
			WHERE source_account_id = $1
		),
		positions AS (
			SELECT
				COUNT(*) as total,
				COUNT(*) FILTER (WHERE completed = TRUE) as completed,
				COUNT(*) FILTER (WHERE completed = FALSE AND progress_percent > 1) as in_progress,
				MAX(updated_at) as last_activity
			FROM np_progress_positions
			WHERE source_account_id = $1
		),
		watchlist AS (
			SELECT COUNT(*) as count
			FROM np_progress_watchlists
			WHERE source_account_id = $1
		),
		favorites AS (
			SELECT COUNT(*) as count
			FROM np_progress_favorites
			WHERE source_account_id = $1
		),
		history AS (
			SELECT COUNT(*) as count
			FROM np_progress_history
			WHERE source_account_id = $1
		)
		SELECT
			u.count,
			p.total,
			p.completed,
			p.in_progress,
			w.count,
			f.count,
			h.count,
			p.last_activity
		FROM users u
		CROSS JOIN positions p
		CROSS JOIN watchlist w
		CROSS JOIN favorites f
		CROSS JOIN history h`,
		d.sourceAccountID,
	).Scan(&totalUsers, &totalPositions, &totalCompleted, &totalInProgress,
		&totalWatchlist, &totalFavorites, &totalHistoryEvts, &lastActivity)
	if err != nil {
		return nil, fmt.Errorf("get plugin stats: %w", err)
	}

	return &PluginStats{
		TotalUsers:       totalUsers,
		TotalPositions:   totalPositions,
		TotalCompleted:   totalCompleted,
		TotalInProgress:  totalInProgress,
		TotalWatchlist:   totalWatchlist,
		TotalFavorites:   totalFavorites,
		TotalHistoryEvts: totalHistoryEvts,
		LastActivity:     lastActivity,
	}, nil
}

