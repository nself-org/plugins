package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with torrent-manager table operations.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// InitSchema creates all tables, indexes, and views if they do not exist.
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
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE VIEW torrent_active_downloads AS
		SELECT * FROM np_torrentmanager_torrent_downloads
		WHERE status IN ('downloading', 'paused')
		ORDER BY added_at DESC
	`); err != nil {
		return fmt.Errorf("create active downloads view: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE VIEW torrent_completed_downloads AS
		SELECT * FROM np_torrentmanager_torrent_downloads
		WHERE status = 'completed'
		ORDER BY completed_at DESC
	`); err != nil {
		return fmt.Errorf("create completed downloads view: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE VIEW torrent_seeding_torrents AS
		SELECT * FROM np_torrentmanager_torrent_downloads
		WHERE status = 'seeding'
		ORDER BY completed_at DESC
	`); err != nil {
		return fmt.Errorf("create seeding torrents view: %w", err)
	}

	return tx.Commit(ctx)
}

// ============================================================================
// Client Operations
// ============================================================================

// ListClients returns all configured torrent clients.
func (d *DB) ListClients() ([]TorrentClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, client_type, host, port, username, password_encrypted,
		        is_default, status, last_connected_at, last_error, created_at, updated_at
		 FROM np_torrentmanager_torrent_clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	var clients []TorrentClient
	for rows.Next() {
		var c TorrentClient
		if err := rows.Scan(
			&c.ID, &c.SourceAccountID, &c.ClientType, &c.Host, &c.Port,
			&c.Username, &c.PasswordEncrypted, &c.IsDefault, &c.Status,
			&c.LastConnectedAt, &c.LastError, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan client: %w", err)
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

// GetDefaultClient returns the default torrent client, or nil if none.
func (d *DB) GetDefaultClient() (*TorrentClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var c TorrentClient
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, client_type, host, port, username, password_encrypted,
		        is_default, status, last_connected_at, last_error, created_at, updated_at
		 FROM np_torrentmanager_torrent_clients WHERE is_default = TRUE LIMIT 1`,
	).Scan(
		&c.ID, &c.SourceAccountID, &c.ClientType, &c.Host, &c.Port,
		&c.Username, &c.PasswordEncrypted, &c.IsDefault, &c.Status,
		&c.LastConnectedAt, &c.LastError, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get default client: %w", err)
	}
	return &c, nil
}

// ============================================================================
// Download Operations
// ============================================================================

// CreateDownload inserts a new download record.
func (d *DB) CreateDownload(dl *TorrentDownload) (*TorrentDownload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metadata := dl.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	var out TorrentDownload
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_torrentmanager_torrent_downloads (
			source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, download_path, requested_by, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
		          status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
		          ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
		          download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
		          error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
		          stopped_at, created_at, updated_at`,
		coalesce(dl.SourceAccountID, "primary"),
		dl.ClientID,
		dl.ClientTorrentID,
		dl.Name,
		dl.InfoHash,
		dl.MagnetURI,
		coalesce(dl.Status, "queued"),
		coalesce(dl.Category, "other"),
		dl.SizeBytes,
		dl.DownloadPath,
		dl.RequestedBy,
		metadata,
	).Scan(
		&out.ID, &out.SourceAccountID, &out.ClientID, &out.ClientTorrentID, &out.Name, &out.InfoHash, &out.MagnetURI,
		&out.Status, &out.Category, &out.SizeBytes, &out.DownloadedBytes, &out.UploadedBytes, &out.ProgressPercent,
		&out.Ratio, &out.DownloadSpeed, &out.UploadSpeed, &out.Seeders, &out.Leechers, &out.PeersConnected,
		&out.DownloadPath, &out.FilesCount, &out.StopAtRatio, &out.StopAtTimeHours, &out.VPNIP, &out.VPNInterface,
		&out.ErrorMessage, &out.ContentID, &out.RequestedBy, &out.Metadata, &out.AddedAt, &out.StartedAt, &out.CompletedAt,
		&out.StoppedAt, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create download: %w", err)
	}
	return &out, nil
}

// GetDownload returns a single download by ID, or nil.
func (d *DB) GetDownload(id string) (*TorrentDownload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dl TorrentDownload
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
		        status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
		        ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
		        download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
		        error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
		        stopped_at, created_at, updated_at
		 FROM np_torrentmanager_torrent_downloads WHERE id = $1`, id,
	).Scan(
		&dl.ID, &dl.SourceAccountID, &dl.ClientID, &dl.ClientTorrentID, &dl.Name, &dl.InfoHash, &dl.MagnetURI,
		&dl.Status, &dl.Category, &dl.SizeBytes, &dl.DownloadedBytes, &dl.UploadedBytes, &dl.ProgressPercent,
		&dl.Ratio, &dl.DownloadSpeed, &dl.UploadSpeed, &dl.Seeders, &dl.Leechers, &dl.PeersConnected,
		&dl.DownloadPath, &dl.FilesCount, &dl.StopAtRatio, &dl.StopAtTimeHours, &dl.VPNIP, &dl.VPNInterface,
		&dl.ErrorMessage, &dl.ContentID, &dl.RequestedBy, &dl.Metadata, &dl.AddedAt, &dl.StartedAt, &dl.CompletedAt,
		&dl.StoppedAt, &dl.CreatedAt, &dl.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get download: %w", err)
	}
	return &dl, nil
}

// ListDownloads returns downloads matching optional filters.
func (d *DB) ListDownloads(status, category string, limit int) ([]TorrentDownload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
	                  status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
	                  ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
	                  download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
	                  error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
	                  stopped_at, created_at, updated_at
	           FROM np_torrentmanager_torrent_downloads WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if category != "" {
		query += fmt.Sprintf(" AND category = $%d", idx)
		args = append(args, category)
		idx++
	}

	query += " ORDER BY added_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", idx)
		args = append(args, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list downloads: %w", err)
	}
	defer rows.Close()

	var downloads []TorrentDownload
	for rows.Next() {
		var dl TorrentDownload
		if err := rows.Scan(
			&dl.ID, &dl.SourceAccountID, &dl.ClientID, &dl.ClientTorrentID, &dl.Name, &dl.InfoHash, &dl.MagnetURI,
			&dl.Status, &dl.Category, &dl.SizeBytes, &dl.DownloadedBytes, &dl.UploadedBytes, &dl.ProgressPercent,
			&dl.Ratio, &dl.DownloadSpeed, &dl.UploadSpeed, &dl.Seeders, &dl.Leechers, &dl.PeersConnected,
			&dl.DownloadPath, &dl.FilesCount, &dl.StopAtRatio, &dl.StopAtTimeHours, &dl.VPNIP, &dl.VPNInterface,
			&dl.ErrorMessage, &dl.ContentID, &dl.RequestedBy, &dl.Metadata, &dl.AddedAt, &dl.StartedAt, &dl.CompletedAt,
			&dl.StoppedAt, &dl.CreatedAt, &dl.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan download: %w", err)
		}
		downloads = append(downloads, dl)
	}
	return downloads, rows.Err()
}

// UpdateDownloadStatus updates the status and optional error message of a download.
func (d *DB) UpdateDownloadStatus(id, status string, errorMessage *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`UPDATE np_torrentmanager_torrent_downloads
		 SET status = $1, error_message = $2, updated_at = NOW()
		 WHERE id = $3`,
		status, errorMessage, id)
	if err != nil {
		return fmt.Errorf("update download status: %w", err)
	}
	return nil
}

// DeleteDownload removes a download by ID.
func (d *DB) DeleteDownload(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`DELETE FROM np_torrentmanager_torrent_downloads WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete download: %w", err)
	}
	return nil
}

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

// ============================================================================
// Statistics
// ============================================================================

// GetStats returns aggregated download statistics.
func (d *DB) GetStats() (*TorrentStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var s TorrentStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status NOT IN ('removed')) as total_downloads,
			COUNT(*) FILTER (WHERE status IN ('downloading', 'paused')) as active_downloads,
			COUNT(*) FILTER (WHERE status = 'completed') as completed_downloads,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_downloads,
			COUNT(*) FILTER (WHERE status = 'seeding') as seeding_torrents,
			COALESCE(SUM(downloaded_bytes), 0) as total_downloaded_bytes,
			COALESCE(SUM(uploaded_bytes), 0) as total_uploaded_bytes,
			COALESCE(AVG(ratio), 0) as overall_ratio,
			COALESCE(SUM(download_speed_bytes), 0) as download_speed_bytes,
			COALESCE(SUM(upload_speed_bytes), 0) as upload_speed_bytes
		FROM np_torrentmanager_torrent_downloads
	`).Scan(
		&s.TotalDownloads, &s.ActiveDownloads, &s.CompletedDownloads,
		&s.FailedDownloads, &s.SeedingTorrents,
		&s.TotalDownloaded, &s.TotalUploaded, &s.OverallRatio,
		&s.DownloadSpeed, &s.UploadSpeed,
	)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	return &s, nil
}

// coalesce returns val if non-empty, otherwise fallback.
func coalesce(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
