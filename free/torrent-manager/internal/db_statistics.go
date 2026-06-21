package internal

import (
	"context"
	"fmt"
	"time"
)

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

