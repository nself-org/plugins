package internal

import (
	"context"
	"strconv"
)

// =========================================================================
// Acquisition History
// =========================================================================

// ListAcquisitionHistory returns history items within the last N days.
func (d *DB) ListAcquisitionHistory(accountID string, days int) ([]AcquisitionHistoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, content_type, content_name,
		   year, season, episode, torrent_title, torrent_source,
		   quality, size_bytes, download_id, status, acquired_from,
		   upgrade_of, created_at
		 FROM np_contentacquisition_acquisition_history
		 WHERE source_account_id = $1
		   AND created_at >= NOW() - ($2 || ' days')::INTERVAL
		 ORDER BY created_at DESC`,
		accountID, strconv.Itoa(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AcquisitionHistoryItem
	for rows.Next() {
		var h AcquisitionHistoryItem
		if err := rows.Scan(
			&h.ID, &h.SourceAccountID, &h.ContentType, &h.ContentName,
			&h.Year, &h.Season, &h.Episode, &h.TorrentTitle, &h.TorrentSource,
			&h.Quality, &h.SizeBytes, &h.DownloadID, &h.Status, &h.AcquiredFrom,
			&h.UpgradeOf, &h.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

