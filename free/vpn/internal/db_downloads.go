package internal

import (
	"context"
	"encoding/json"
	pgx "github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Download operations
// ---------------------------------------------------------------------------

// CreateDownload inserts a new download record.
func (d *DB) CreateDownload(ctx context.Context, dl *Download) error {
	metaJSON, _ := json.Marshal(dl.Metadata)
	return d.pool.QueryRow(ctx,
		`INSERT INTO np_vpn_downloads (
			connection_id, magnet_link, info_hash, name, destination_path, status, progress,
			bytes_downloaded, bytes_total, requested_by, provider_id, server_id, metadata, source_account_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at`,
		dl.ConnectionID, dl.MagnetLink, dl.InfoHash, dl.Name, dl.DestinationPath,
		coalesceStr(dl.Status, "queued"), dl.Progress, dl.BytesDownloaded, dl.BytesTotal,
		dl.RequestedBy, dl.ProviderID, dl.ServerID, metaJSON, d.sourceAccountID,
	).Scan(&dl.ID, &dl.CreatedAt)
}

// GetDownload returns a single download by ID.
func (d *DB) GetDownload(ctx context.Context, id string) (*Download, error) {
	var dl Download
	var metaJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, connection_id, magnet_link, info_hash, name, destination_path, status,
			progress, bytes_downloaded, bytes_total, download_speed, upload_speed, peers, seeds,
			eta_seconds, requested_by, provider_id, server_id, started_at, completed_at,
			error_message, metadata, source_account_id, created_at
		FROM np_vpn_downloads WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(
		&dl.ID, &dl.ConnectionID, &dl.MagnetLink, &dl.InfoHash, &dl.Name,
		&dl.DestinationPath, &dl.Status, &dl.Progress, &dl.BytesDownloaded,
		&dl.BytesTotal, &dl.DownloadSpeed, &dl.UploadSpeed, &dl.Peers, &dl.Seeds,
		&dl.ETASeconds, &dl.RequestedBy, &dl.ProviderID, &dl.ServerID,
		&dl.StartedAt, &dl.CompletedAt, &dl.ErrorMessage, &metaJSON,
		&dl.SourceAccountID, &dl.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dl.Metadata = make(map[string]interface{})
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &dl.Metadata)
	}
	return &dl, nil
}

// GetAllDownloads returns downloads scoped to the current account.
func (d *DB) GetAllDownloads(ctx context.Context, limit int) ([]Download, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.pool.Query(ctx,
		`SELECT id, connection_id, magnet_link, info_hash, name, destination_path, status,
			progress, bytes_downloaded, bytes_total, download_speed, upload_speed, peers, seeds,
			eta_seconds, requested_by, provider_id, server_id, started_at, completed_at,
			error_message, metadata, source_account_id, created_at
		FROM np_vpn_downloads WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2`,
		d.sourceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Download
	for rows.Next() {
		var dl Download
		var metaJSON []byte
		if err := rows.Scan(
			&dl.ID, &dl.ConnectionID, &dl.MagnetLink, &dl.InfoHash, &dl.Name,
			&dl.DestinationPath, &dl.Status, &dl.Progress, &dl.BytesDownloaded,
			&dl.BytesTotal, &dl.DownloadSpeed, &dl.UploadSpeed, &dl.Peers, &dl.Seeds,
			&dl.ETASeconds, &dl.RequestedBy, &dl.ProviderID, &dl.ServerID,
			&dl.StartedAt, &dl.CompletedAt, &dl.ErrorMessage, &metaJSON,
			&dl.SourceAccountID, &dl.CreatedAt,
		); err != nil {
			return nil, err
		}
		dl.Metadata = make(map[string]interface{})
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &dl.Metadata)
		}
		result = append(result, dl)
	}
	return result, rows.Err()
}

// UpdateDownloadStatus sets the status and optional error message on a download.
func (d *DB) UpdateDownloadStatus(ctx context.Context, id, status string, errMsg *string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE np_vpn_downloads SET status = $1, error_message = $2
		WHERE id = $3 AND source_account_id = $4`,
		status, errMsg, id, d.sourceAccountID)
	return err
}

