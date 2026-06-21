package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// =========================================================================
// Progress Positions
// =========================================================================

// UpdateProgress upserts a playback position and samples a history event.
func (d *DB) UpdateProgress(req UpdateProgressRequest) (*ProgressPosition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var progressPercent float64
	if req.DurationSeconds != nil && *req.DurationSeconds > 0 {
		progressPercent = (req.PositionSeconds / *req.DurationSeconds) * 100
	}

	completed := progressPercent >= float64(d.completeThreshold)
	var completedAt *time.Time
	if completed {
		now := time.Now()
		completedAt = &now
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		metadataBytes = []byte("{}")
	}
	if req.Metadata == nil {
		metadataBytes = []byte("{}")
	}

	var pos ProgressPosition
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_progress_positions (
			source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent,
			completed, completed_at, device_id, audio_track, subtitle_track,
			quality, metadata, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
		ON CONFLICT (source_account_id, user_id, content_type, content_id)
		DO UPDATE SET
			position_seconds = EXCLUDED.position_seconds,
			duration_seconds = COALESCE(EXCLUDED.duration_seconds, np_progress_positions.duration_seconds),
			progress_percent = EXCLUDED.progress_percent,
			completed = EXCLUDED.completed,
			completed_at = CASE
				WHEN EXCLUDED.completed AND np_progress_positions.completed_at IS NULL
				THEN EXCLUDED.completed_at
				ELSE np_progress_positions.completed_at
			END,
			device_id = COALESCE(EXCLUDED.device_id, np_progress_positions.device_id),
			audio_track = COALESCE(EXCLUDED.audio_track, np_progress_positions.audio_track),
			subtitle_track = COALESCE(EXCLUDED.subtitle_track, np_progress_positions.subtitle_track),
			quality = COALESCE(EXCLUDED.quality, np_progress_positions.quality),
			metadata = EXCLUDED.metadata,
			updated_at = NOW()
		RETURNING id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent,
			completed, completed_at, device_id, audio_track, subtitle_track,
			quality, metadata, updated_at, created_at`,
		d.sourceAccountID,
		req.UserID,
		req.ContentType,
		req.ContentID,
		req.PositionSeconds,
		req.DurationSeconds,
		progressPercent,
		completed,
		completedAt,
		req.DeviceID,
		req.AudioTrack,
		req.SubtitleTrack,
		req.Quality,
		metadataBytes,
	).Scan(
		&pos.ID, &pos.SourceAccountID, &pos.UserID, &pos.ContentType, &pos.ContentID,
		&pos.PositionSeconds, &pos.DurationSeconds, &pos.ProgressPercent,
		&pos.Completed, &pos.CompletedAt, &pos.DeviceID, &pos.AudioTrack, &pos.SubtitleTrack,
		&pos.Quality, &pos.Metadata, &pos.UpdatedAt, &pos.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update progress: %w", err)
	}

	// Record history event with sampling
	d.maybeSampleHistoryEvent(req.UserID, req.ContentType, req.ContentID, string(ActionPlay), req.PositionSeconds, req.DeviceID)

	return &pos, nil
}

// maybeSampleHistoryEvent inserts a history row only if enough time has elapsed
// since the last insert for the same user+content key (throttled by historySampleSeconds).
func (d *DB) maybeSampleHistoryEvent(userID, contentType, contentID, action string, positionSeconds float64, deviceID *string) {
	key := userID + ":" + contentType + ":" + contentID
	now := time.Now()

	d.mu.Lock()
	lastInsert, exists := d.lastHistoryInsert[key]
	elapsed := now.Sub(lastInsert).Seconds()
	shouldInsert := !exists || elapsed >= float64(d.historySampleSeconds)
	if shouldInsert {
		d.lastHistoryInsert[key] = now
	}
	d.mu.Unlock()

	if shouldInsert {
		// Best-effort: do not block on history insert failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = d.pool.Exec(ctx,
			`INSERT INTO np_progress_history (
				source_account_id, user_id, content_type, content_id,
				action, position_seconds, device_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			d.sourceAccountID, userID, contentType, contentID, action, positionSeconds, deviceID,
		)
	}
}
