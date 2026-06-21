package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Acquisition Queue
// =========================================================================

func scanQueueItem(row pgx.Row) (*AcquisitionQueueItem, error) {
	var q AcquisitionQueueItem
	err := row.Scan(
		&q.ID, &q.SourceAccountID, &q.ContentType, &q.ContentName,
		&q.Year, &q.Season, &q.Episode, &q.QualityProfileID,
		&q.RequestedBy, &q.RequestSourceID, &q.Status, &q.Priority,
		&q.Attempts, &q.MaxAttempts, &q.MatchedTorrent, &q.DownloadID,
		&q.ErrorMessage, &q.CreatedAt, &q.StartedAt, &q.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

const queueColumns = `id, source_account_id, content_type, content_name,
  year, season, episode, quality_profile_id,
  requested_by, request_source_id, status, priority,
  attempts, max_attempts, matched_torrent, download_id,
  error_message, created_at, started_at, completed_at`

// GetQueue returns active queue items for an account.
func (d *DB) GetQueue(accountID string) ([]AcquisitionQueueItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT %s FROM np_contentacquisition_acquisition_queue
			 WHERE source_account_id = $1
			   AND status IN ('pending', 'searching', 'matched', 'downloading')
			 ORDER BY priority DESC, created_at ASC`, queueColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AcquisitionQueueItem
	for rows.Next() {
		var q AcquisitionQueueItem
		if err := rows.Scan(
			&q.ID, &q.SourceAccountID, &q.ContentType, &q.ContentName,
			&q.Year, &q.Season, &q.Episode, &q.QualityProfileID,
			&q.RequestedBy, &q.RequestSourceID, &q.Status, &q.Priority,
			&q.Attempts, &q.MaxAttempts, &q.MatchedTorrent, &q.DownloadID,
			&q.ErrorMessage, &q.CreatedAt, &q.StartedAt, &q.CompletedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, q)
	}
	return items, rows.Err()
}

// AddToQueue inserts a new item into the acquisition queue.
func (d *DB) AddToQueue(accountID, contentType, contentName string, year, season, episode *int, requestedBy string) (*AcquisitionQueueItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_acquisition_queue
			   (source_account_id, content_type, content_name, year, season, episode,
			    requested_by, priority)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 5)
			 RETURNING %s`, queueColumns),
		accountID, contentType, contentName, year, season, episode, requestedBy,
	)
	return scanQueueItem(row)
}

