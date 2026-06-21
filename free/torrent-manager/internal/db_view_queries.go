package internal

import (
	"context"
	"fmt"
	"time"
	pgx "github.com/jackc/pgx/v5"
)

// ============================================================================
// View Queries with Limit
// ============================================================================

// GetActiveDownloads returns active downloads (downloading or paused) with LIMIT applied.
func (d *DB) GetActiveDownloads() ([]TorrentDownload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT
			id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
			ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
			download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
			error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
			stopped_at, created_at, updated_at
		FROM torrent_active_downloads LIMIT $1`, d.torrentListLimit)
	if err != nil {
		return nil, fmt.Errorf("get active downloads: %w", err)
	}
	defer rows.Close()

	return scanDownloads(rows)
}

// GetCompletedDownloads returns completed downloads with LIMIT applied.
func (d *DB) GetCompletedDownloads() ([]TorrentDownload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT
			id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
			ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
			download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
			error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
			stopped_at, created_at, updated_at
		FROM torrent_completed_downloads LIMIT $1`, d.torrentListLimit)
	if err != nil {
		return nil, fmt.Errorf("get completed downloads: %w", err)
	}
	defer rows.Close()

	return scanDownloads(rows)
}

// GetSeedingTorrents returns seeding torrents with LIMIT applied.
func (d *DB) GetSeedingTorrents() ([]TorrentDownload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT
			id, source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
			status, category, size_bytes, downloaded_bytes, uploaded_bytes, progress_percent,
			ratio, download_speed_bytes, upload_speed_bytes, seeders, leechers, peers_connected,
			download_path, files_count, stop_at_ratio, stop_at_time_hours, vpn_ip, vpn_interface,
			error_message, content_id, requested_by, metadata, added_at, started_at, completed_at,
			stopped_at, created_at, updated_at
		FROM torrent_seeding_torrents LIMIT $1`, d.torrentListLimit)
	if err != nil {
		return nil, fmt.Errorf("get seeding torrents: %w", err)
	}
	defer rows.Close()

	return scanDownloads(rows)
}

// scanDownloads is a helper to scan download rows.
func scanDownloads(rows pgx.Rows) ([]TorrentDownload, error) {
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

// coalesce returns val if non-empty, otherwise fallback.
func coalesce(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

