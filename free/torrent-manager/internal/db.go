package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with torrent-manager table operations.
type DB struct {
	pool             *pgxpool.Pool
	torrentListLimit int
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, torrentListLimit: 200}
}

// NewDBWithLimit creates a new DB wrapper with a custom torrent list limit.
func NewDBWithLimit(pool *pgxpool.Pool, limit int) *DB {
	if limit <= 0 {
		limit = 200
	}
	return &DB{pool: pool, torrentListLimit: limit}
}

// InitSchema creates all tables, indexes, and views if they do not exist.
// Size-cap exception: SQL DDL migration — 361L of linear SQL statements; splitting across files adds no value and breaks transactional migration semantics.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Torrent Clients Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_clients (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			client_type VARCHAR(50) NOT NULL,
			host VARCHAR(255) NOT NULL,
			port INT NOT NULL,
			username VARCHAR(255),
			password_encrypted TEXT,
			is_default BOOLEAN DEFAULT FALSE,
			status VARCHAR(50) NOT NULL DEFAULT 'disconnected',
			last_connected_at TIMESTAMPTZ,
			last_error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create clients table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_clients_account
		ON np_torrentmanager_torrent_clients(source_account_id)
	`); err != nil {
		return fmt.Errorf("create clients account index: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_clients_type
		ON np_torrentmanager_torrent_clients(client_type)
	`); err != nil {
		return fmt.Errorf("create clients type index: %w", err)
	}

	// Torrent Sources Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_sources (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			source_name VARCHAR(50) NOT NULL,
			base_url VARCHAR(500) NOT NULL,
			is_active BOOLEAN DEFAULT TRUE,
			priority INT DEFAULT 50,
			requires_proxy BOOLEAN DEFAULT FALSE,
			last_success_at TIMESTAMPTZ,
			last_failure_at TIMESTAMPTZ,
			failure_count INT DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create sources table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_sources_active
		ON np_torrentmanager_sources(is_active) WHERE is_active = TRUE
	`); err != nil {
		return fmt.Errorf("create sources active index: %w", err)
	}

	// Torrent Downloads Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_downloads (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			client_id UUID NOT NULL REFERENCES np_torrentmanager_torrent_clients(id) ON DELETE CASCADE,
			client_torrent_id VARCHAR(255) NOT NULL,
			name VARCHAR(500) NOT NULL,
			info_hash VARCHAR(40) NOT NULL,
			magnet_uri TEXT NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'queued',
			category VARCHAR(50) NOT NULL DEFAULT 'other',
			size_bytes BIGINT DEFAULT 0,
			downloaded_bytes BIGINT DEFAULT 0,
			uploaded_bytes BIGINT DEFAULT 0,
			progress_percent DECIMAL(5,2) DEFAULT 0,
			ratio DECIMAL(5,2) DEFAULT 0,
			download_speed_bytes BIGINT DEFAULT 0,
			upload_speed_bytes BIGINT DEFAULT 0,
			seeders INT DEFAULT 0,
			leechers INT DEFAULT 0,
			peers_connected INT DEFAULT 0,
			download_path VARCHAR(500),
			files_count INT DEFAULT 0,
			stop_at_ratio DECIMAL(5,2),
			stop_at_time_hours INT,
			vpn_ip VARCHAR(50),
			vpn_interface VARCHAR(50),
			error_message TEXT,
			content_id UUID,
			requested_by VARCHAR(255) NOT NULL,
			metadata JSONB DEFAULT '{}',
			added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			stopped_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create downloads table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_downloads_account
		ON np_torrentmanager_torrent_downloads(source_account_id)
	`); err != nil {
		return fmt.Errorf("create downloads account index: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_downloads_status
		ON np_torrentmanager_torrent_downloads(status)
	`); err != nil {
		return fmt.Errorf("create downloads status index: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_downloads_info_hash
		ON np_torrentmanager_torrent_downloads(info_hash)
	`); err != nil {
		return fmt.Errorf("create downloads hash index: %w", err)
	}

	// Torrent Files Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_files (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			download_id UUID NOT NULL REFERENCES np_torrentmanager_torrent_downloads(id) ON DELETE CASCADE,
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			file_index INT NOT NULL,
			file_name VARCHAR(500) NOT NULL,
			file_path VARCHAR(500) NOT NULL,
			size_bytes BIGINT NOT NULL,
			downloaded_bytes BIGINT DEFAULT 0,
			progress_percent DECIMAL(5,2) DEFAULT 0,
			priority INT DEFAULT 0,
			is_selected BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create files table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_files_download
		ON np_torrentmanager_torrent_files(download_id)
	`); err != nil {
		return fmt.Errorf("create files download index: %w", err)
	}

	// Torrent Trackers Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_trackers (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			download_id UUID NOT NULL REFERENCES np_torrentmanager_torrent_downloads(id) ON DELETE CASCADE,
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			tracker_url VARCHAR(500) NOT NULL,
			tier INT NOT NULL,
			status VARCHAR(50) NOT NULL,
			seeders INT,
			leechers INT,
			last_announce_at TIMESTAMPTZ,
			last_scrape_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create trackers table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_torrent_trackers_download
		ON np_torrentmanager_torrent_trackers(download_id)
	`); err != nil {
		return fmt.Errorf("create trackers download index: %w", err)
	}

	// Torrent Search Cache Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_search_cache (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			query_hash VARCHAR(64) NOT NULL,
			query TEXT NOT NULL,
			results JSONB NOT NULL DEFAULT '[]',
			results_count INT DEFAULT 0,
			sources_searched VARCHAR(50)[] DEFAULT '{}',
			search_duration_ms INT,
			cached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create search cache table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_search_cache_hash
		ON np_torrentmanager_search_cache(query_hash)
	`); err != nil {
		return fmt.Errorf("create search cache hash index: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_search_cache_expires
		ON np_torrentmanager_search_cache(expires_at)
	`); err != nil {
		return fmt.Errorf("create search cache expires index: %w", err)
	}

	// Seeding Policy Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_seeding_policy (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			policy_name VARCHAR(255) NOT NULL,
			description TEXT,
			ratio_limit DECIMAL(5,2),
			ratio_action VARCHAR(50) DEFAULT 'stop',
			time_limit_hours INT,
			time_action VARCHAR(50) DEFAULT 'stop',
			max_seeding_size_gb INT,
			applies_to_categories VARCHAR(50)[] DEFAULT '{}',
			priority INT DEFAULT 50,
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create seeding policy table: %w", err)
	}

	// Torrent Stats Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_stats (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			total_downloads INT DEFAULT 0,
			active_downloads INT DEFAULT 0,
			completed_downloads INT DEFAULT 0,
			failed_downloads INT DEFAULT 0,
			seeding_torrents INT DEFAULT 0,
			total_downloaded_bytes BIGINT DEFAULT 0,
			total_uploaded_bytes BIGINT DEFAULT 0,
			overall_ratio DECIMAL(5,2) DEFAULT 0,
			download_speed_bytes BIGINT DEFAULT 0,
			upload_speed_bytes BIGINT DEFAULT 0,
			disk_space_used_bytes BIGINT DEFAULT 0,
			disk_space_available_bytes BIGINT DEFAULT 0,
			snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create stats table: %w", err)
	}

	// Per-Download Seeding Policies Table
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_torrentmanager_seeding_policies (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
			download_id VARCHAR(255) NOT NULL,
			ratio_limit DECIMAL(5,2) DEFAULT 2.0,
			time_limit_hours INT DEFAULT 168,
			auto_remove BOOLEAN DEFAULT TRUE,
			keep_files BOOLEAN DEFAULT FALSE,
			favorite BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create seeding policies table: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_torrentmanager_seeding_policies_download
		ON np_torrentmanager_seeding_policies(download_id)
	`); err != nil {
		return fmt.Errorf("create seeding policies download index: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_seeding_policies_account
		ON np_torrentmanager_seeding_policies(source_account_id)
	`); err != nil {
		return fmt.Errorf("create seeding policies account index: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_seeding_policies_favorite
		ON np_torrentmanager_seeding_policies(favorite) WHERE favorite = TRUE
	`); err != nil {
		return fmt.Errorf("create seeding policies favorite index: %w", err)
	}

	// Views
	// Note: LIMIT is applied at query time, not in the view definition
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE VIEW torrent_active_downloads AS
		SELECT
			id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
			ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
			download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
			error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
			stopped_at, created_at, updated_at
		FROM np_torrentmanager_torrent_downloads
		WHERE status IN ('downloading', 'paused')
		ORDER BY added_at DESC
	`); err != nil {
		return fmt.Errorf("create active downloads view: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE VIEW torrent_completed_downloads AS
		SELECT
			id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
			ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
			download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
			error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
			stopped_at, created_at, updated_at
		FROM np_torrentmanager_torrent_downloads
		WHERE status = 'completed'
		ORDER BY completed_at DESC
	`); err != nil {
		return fmt.Errorf("create completed downloads view: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE VIEW torrent_seeding_torrents AS
		SELECT
			id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
			ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
			download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
			error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
			stopped_at, created_at, updated_at
		FROM np_torrentmanager_torrent_downloads
		WHERE status = 'seeding'
		ORDER BY completed_at DESC
	`); err != nil {
		return fmt.Errorf("create seeding torrents view: %w", err)
	}

	return tx.Commit(ctx)
}

