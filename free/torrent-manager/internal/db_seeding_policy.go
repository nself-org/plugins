package internal

import (
	"context"
	"fmt"
	"time"
	pgx "github.com/jackc/pgx/v5"
)

// ============================================================================
// Seeding Policy Operations
// ============================================================================

// UpsertDownloadSeedingPolicy creates or updates a per-download seeding policy.
func (d *DB) UpsertDownloadSeedingPolicy(downloadID string, req SeedingConfigRequest, sourceAccountID string) (*DownloadSeedingPolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ratioLimit := 2.0
	if req.RatioLimit != nil {
		ratioLimit = *req.RatioLimit
	}
	timeLimitHours := 168
	if req.TimeLimitHours != nil {
		timeLimitHours = *req.TimeLimitHours
	}
	autoRemove := true
	if req.AutoRemove != nil {
		autoRemove = *req.AutoRemove
	}
	keepFiles := false
	if req.KeepFiles != nil {
		keepFiles = *req.KeepFiles
	}
	favorite := false
	if req.Favorite != nil {
		favorite = *req.Favorite
	}

	// Favorites must never be auto-removed
	if favorite {
		autoRemove = false
	}

	var p DownloadSeedingPolicy
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_torrentmanager_seeding_policies (
			source_account_id, download_id, ratio_limit, time_limit_hours,
			auto_remove, keep_files, favorite
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (download_id) DO UPDATE SET
			ratio_limit = COALESCE(EXCLUDED.ratio_limit, np_torrentmanager_seeding_policies.ratio_limit),
			time_limit_hours = COALESCE(EXCLUDED.time_limit_hours, np_torrentmanager_seeding_policies.time_limit_hours),
			auto_remove = COALESCE(EXCLUDED.auto_remove, np_torrentmanager_seeding_policies.auto_remove),
			keep_files = COALESCE(EXCLUDED.keep_files, np_torrentmanager_seeding_policies.keep_files),
			favorite = COALESCE(EXCLUDED.favorite, np_torrentmanager_seeding_policies.favorite),
			updated_at = NOW()
		RETURNING id, source_account_id, download_id, ratio_limit, time_limit_hours,
		          auto_remove, keep_files, favorite, created_at, updated_at`,
		sourceAccountID, downloadID, ratioLimit, timeLimitHours, autoRemove, keepFiles, favorite,
	).Scan(
		&p.ID, &p.SourceAccountID, &p.DownloadID, &p.RatioLimit, &p.TimeLimitHours,
		&p.AutoRemove, &p.KeepFiles, &p.Favorite, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert seeding policy: %w", err)
	}
	return &p, nil
}

// GetDownloadSeedingPolicy returns the seeding policy for a download, or nil.
func (d *DB) GetDownloadSeedingPolicy(downloadID string) (*DownloadSeedingPolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var p DownloadSeedingPolicy
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, download_id, ratio_limit, time_limit_hours,
		        auto_remove, keep_files, favorite, created_at, updated_at
		 FROM np_torrentmanager_seeding_policies WHERE download_id = $1`,
		downloadID,
	).Scan(
		&p.ID, &p.SourceAccountID, &p.DownloadID, &p.RatioLimit, &p.TimeLimitHours,
		&p.AutoRemove, &p.KeepFiles, &p.Favorite, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get seeding policy: %w", err)
	}
	return &p, nil
}

