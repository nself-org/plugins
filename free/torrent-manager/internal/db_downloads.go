package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	pgx "github.com/jackc/pgx/v5"
)

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
// Size-cap exception: single DB operation — 55L scan loop with struct mapping; splitting would fragment a single SQL query across files.
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

