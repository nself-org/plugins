package internal

import (
	"context"
	"fmt"
	"time"
	pgx "github.com/jackc/pgx/v5"
)

// ============================================================================
// Search Cache Operations
// ============================================================================

// GetSearchCache returns a cached search result by query hash, or nil if expired/not found.
func (d *DB) GetSearchCache(queryHash string) (*TorrentSearchCache, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sc TorrentSearchCache
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, query_hash, query, results, results_count,
		        sources_searched, search_duration_ms, cached_at, expires_at, created_at
		 FROM np_torrentmanager_search_cache
		 WHERE query_hash = $1 AND expires_at > NOW()
		 ORDER BY created_at DESC LIMIT 1`,
		queryHash,
	).Scan(
		&sc.ID, &sc.SourceAccountID, &sc.QueryHash, &sc.Query, &sc.Results, &sc.ResultsCount,
		&sc.SourcesSearched, &sc.SearchDurationMS, &sc.CachedAt, &sc.ExpiresAt, &sc.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get search cache: %w", err)
	}
	return &sc, nil
}

