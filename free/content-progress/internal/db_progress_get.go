package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"fmt"
	"time"
)

// GetProgress returns a single position record or nil if not found.
func (d *DB) GetProgress(userID, contentType, contentID string) (*ProgressPosition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pos ProgressPosition
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent,
			completed, completed_at, device_id, audio_track, subtitle_track,
			quality, metadata, updated_at, created_at
		FROM np_progress_positions
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
		d.sourceAccountID, userID, contentType, contentID,
	).Scan(
		&pos.ID, &pos.SourceAccountID, &pos.UserID, &pos.ContentType, &pos.ContentID,
		&pos.PositionSeconds, &pos.DurationSeconds, &pos.ProgressPercent,
		&pos.Completed, &pos.CompletedAt, &pos.DeviceID, &pos.AudioTrack, &pos.SubtitleTrack,
		&pos.Quality, &pos.Metadata, &pos.UpdatedAt, &pos.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get progress: %w", err)
	}
	return &pos, nil
}

// GetUserProgress returns all positions for a user, ordered by updated_at DESC.
func (d *DB) GetUserProgress(userID string, limit, offset int) ([]ProgressPosition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent,
			completed, completed_at, device_id, audio_track, subtitle_track,
			quality, metadata, updated_at, created_at
		FROM np_progress_positions
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get user progress: %w", err)
	}
	defer rows.Close()

	return scanPositions(rows)
}

// DeleteProgress removes a position record. Returns true if a row was deleted.
func (d *DB) DeleteProgress(userID, contentType, contentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_progress_positions
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
		d.sourceAccountID, userID, contentType, contentID,
	)
	if err != nil {
		return false, fmt.Errorf("delete progress: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// MarkCompleted sets a position as completed at 100%.
func (d *DB) MarkCompleted(userID, contentType, contentID string) (*ProgressPosition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pos ProgressPosition
	err := d.pool.QueryRow(ctx,
		`UPDATE np_progress_positions
		SET completed = TRUE,
			completed_at = COALESCE(completed_at, NOW()),
			progress_percent = 100,
			updated_at = NOW()
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4
		RETURNING id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent,
			completed, completed_at, device_id, audio_track, subtitle_track,
			quality, metadata, updated_at, created_at`,
		d.sourceAccountID, userID, contentType, contentID,
	).Scan(
		&pos.ID, &pos.SourceAccountID, &pos.UserID, &pos.ContentType, &pos.ContentID,
		&pos.PositionSeconds, &pos.DurationSeconds, &pos.ProgressPercent,
		&pos.Completed, &pos.CompletedAt, &pos.DeviceID, &pos.AudioTrack, &pos.SubtitleTrack,
		&pos.Quality, &pos.Metadata, &pos.UpdatedAt, &pos.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mark completed: %w", err)
	}

	// Insert a "complete" history event (no sampling)
	d.insertHistoryEvent(userID, contentType, contentID, string(ActionComplete), nil, nil, nil)

	return &pos, nil
}

// GetContinueWatching returns in-progress positions (not completed, >1% and below threshold).
