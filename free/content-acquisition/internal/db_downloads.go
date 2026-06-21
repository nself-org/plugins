package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Downloads
// =========================================================================

const downloadColumns = `id, source_account_id, user_id, content_type, title,
  state, progress, magnet_uri, torrent_id, encoding_job_id,
  quality_profile, retry_count, error_message,
  show_id, season_number, episode_number, tmdb_id,
  created_at, updated_at`

func scanDownload(row pgx.Row) (*Download, error) {
	var dl Download
	err := row.Scan(
		&dl.ID, &dl.SourceAccountID, &dl.UserID, &dl.ContentType, &dl.Title,
		&dl.State, &dl.Progress, &dl.MagnetURI, &dl.TorrentID, &dl.EncodingJobID,
		&dl.QualityProfile, &dl.RetryCount, &dl.ErrorMessage,
		&dl.ShowID, &dl.SeasonNumber, &dl.EpisodeNumber, &dl.TmdbID,
		&dl.CreatedAt, &dl.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &dl, nil
}

// CreateDownload inserts a new download record and its initial state history entry.
func (d *DB) CreateDownload(accountID, contentType, title string, magnetURI *string, qualityProfile string, showID *string, seasonNumber, episodeNumber, tmdbID *int) (*Download, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_downloads
			   (source_account_id, user_id, content_type, title, state,
			    magnet_uri, quality_profile, show_id, season_number, episode_number, tmdb_id)
			 VALUES ($1, $1, $2, $3, 'created', $4, $5, $6, $7, $8, $9)
			 RETURNING %s`, downloadColumns),
		accountID, contentType, title, magnetURI, qualityProfile,
		showID, seasonNumber, episodeNumber, tmdbID,
	)
	dl, err := scanDownload(row)
	if err != nil {
		return nil, err
	}

	// Record initial state
	_, err = tx.Exec(ctx,
		`INSERT INTO np_contentacquisition_download_state_history
		   (download_id, from_state, to_state, metadata)
		 VALUES ($1, NULL, $2, $3)`,
		dl.ID, dl.State, json.RawMessage(`{"source":"creation"}`),
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return dl, nil
}

// GetDownload returns a single download by ID.
func (d *DB) GetDownload(id string) (*Download, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_downloads WHERE id = $1`, downloadColumns), id)
	dl, err := scanDownload(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return dl, err
}

// ListDownloads returns downloads for an account, optionally filtered by state.
func (d *DB) ListDownloads(accountID string, stateFilter *string) ([]Download, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	query := fmt.Sprintf(`SELECT %s FROM np_contentacquisition_downloads WHERE source_account_id = $1`, downloadColumns)
	args := []interface{}{accountID}

	if stateFilter != nil {
		query += ` AND state = $2`
		args = append(args, *stateFilter)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var downloads []Download
	for rows.Next() {
		var dl Download
		if err := rows.Scan(
			&dl.ID, &dl.SourceAccountID, &dl.UserID, &dl.ContentType, &dl.Title,
			&dl.State, &dl.Progress, &dl.MagnetURI, &dl.TorrentID, &dl.EncodingJobID,
			&dl.QualityProfile, &dl.RetryCount, &dl.ErrorMessage,
			&dl.ShowID, &dl.SeasonNumber, &dl.EpisodeNumber, &dl.TmdbID,
			&dl.CreatedAt, &dl.UpdatedAt,
		); err != nil {
			return nil, err
		}
		downloads = append(downloads, dl)
	}
	return downloads, rows.Err()
}

// UpdateDownloadState transitions a download to a new state and records history.
func (d *DB) UpdateDownloadState(id, toState string, meta json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get current state
	var fromState *string
	err = tx.QueryRow(ctx,
		`SELECT state FROM np_contentacquisition_downloads WHERE id = $1`, id).Scan(&fromState)
	if err != nil {
		return err
	}

	// Update state
	_, err = tx.Exec(ctx,
		`UPDATE np_contentacquisition_downloads SET state = $2, updated_at = NOW() WHERE id = $1`,
		id, toState)
	if err != nil {
		return err
	}

	if meta == nil {
		meta = json.RawMessage("{}")
	}

	// Record transition
	_, err = tx.Exec(ctx,
		`INSERT INTO np_contentacquisition_download_state_history
		   (download_id, from_state, to_state, metadata)
		 VALUES ($1, $2, $3, $4)`,
		id, fromState, toState, meta)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateDownloadFields updates specific mutable fields on a download.
func (d *DB) UpdateDownloadFields(id string, retryCount *int, errorMessage *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if retryCount != nil {
		setClauses = append(setClauses, fmt.Sprintf("retry_count = $%d", idx))
		args = append(args, *retryCount)
		idx++
	}
	if errorMessage != nil {
		setClauses = append(setClauses, fmt.Sprintf("error_message = $%d", idx))
		args = append(args, *errorMessage)
		idx++
	}

	if len(args) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_downloads SET %s WHERE id = $%d`,
		strings.Join(setClauses, ", "), idx,
	)
	_, err := d.pool.Exec(ctx, query, args...)
	return err
}

// GetDownloadStateHistory returns all state transitions for a download.
func (d *DB) GetDownloadStateHistory(downloadID string) ([]DownloadStateTransition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, download_id, from_state, to_state, metadata, created_at
		 FROM np_contentacquisition_download_state_history
		 WHERE download_id = $1
		 ORDER BY created_at ASC`,
		downloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []DownloadStateTransition
	for rows.Next() {
		var t DownloadStateTransition
		if err := rows.Scan(&t.ID, &t.DownloadID, &t.FromState, &t.ToState, &t.Metadata, &t.CreatedAt); err != nil {
			return nil, err
		}
		transitions = append(transitions, t)
	}
	return transitions, rows.Err()
}

// AddToDownloadQueue adds or updates a download in the priority queue.
func (d *DB) AddToDownloadQueue(downloadID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_contentacquisition_download_queue (download_id, priority)
		 VALUES ($1, 10)
		 ON CONFLICT (download_id) DO UPDATE SET priority = 10`,
		downloadID)
	return err
}

// RemoveFromDownloadQueue removes a download from the queue.
func (d *DB) RemoveFromDownloadQueue(downloadID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_download_queue WHERE download_id = $1`, downloadID)
	return err
}

