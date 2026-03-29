package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with content-progress table operations.
type DB struct {
	pool                 *pgxpool.Pool
	sourceAccountID      string
	completeThreshold    int
	historySampleSeconds int

	mu                sync.Mutex
	lastHistoryInsert map[string]time.Time
}

// NewDB creates a new DB wrapper with default source account.
func NewDB(pool *pgxpool.Pool, cfg Config) *DB {
	return &DB{
		pool:                 pool,
		sourceAccountID:      "primary",
		completeThreshold:    cfg.CompleteThreshold,
		historySampleSeconds: cfg.HistorySampleSeconds,
		lastHistoryInsert:    make(map[string]time.Time),
	}
}

// ForSourceAccount returns a new DB scoped to the given source account ID.
// It shares the same pool, config, and history sampling state.
func (d *DB) ForSourceAccount(sourceAccountID string) *DB {
	return &DB{
		pool:                 d.pool,
		sourceAccountID:      normalizeSourceAccountID(sourceAccountID),
		completeThreshold:    d.completeThreshold,
		historySampleSeconds: d.historySampleSeconds,
		lastHistoryInsert:    d.lastHistoryInsert,
		mu:                   sync.Mutex{},
	}
}

var nonAlphanumRegex = regexp.MustCompile(`[^a-z0-9_-]+`)
var leadTrailDash = regexp.MustCompile(`^-+|-+$`)

func normalizeSourceAccountID(value string) string {
	normalized := strings.ToLower(value)
	normalized = nonAlphanumRegex.ReplaceAllString(normalized, "-")
	normalized = leadTrailDash.ReplaceAllString(normalized, "")
	if normalized == "" {
		return "primary"
	}
	return normalized
}

// InitSchema creates all tables and indexes if they do not exist.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_progress_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    position_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_seconds DOUBLE PRECISION,
    progress_percent DOUBLE PRECISION DEFAULT 0,
    completed BOOLEAN DEFAULT FALSE,
    completed_at TIMESTAMP WITH TIME ZONE,
    device_id VARCHAR(255),
    audio_track VARCHAR(16),
    subtitle_track VARCHAR(16),
    quality VARCHAR(16),
    metadata JSONB DEFAULT '{}',
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, content_type, content_id)
);

CREATE INDEX IF NOT EXISTS idx_np_progress_positions_source_account
    ON np_progress_positions(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_user
    ON np_progress_positions(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_content
    ON np_progress_positions(content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_completed
    ON np_progress_positions(completed);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_updated
    ON np_progress_positions(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_user_updated
    ON np_progress_positions(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS np_progress_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    action VARCHAR(16) NOT NULL DEFAULT 'play',
    position_seconds DOUBLE PRECISION,
    device_id VARCHAR(255),
    session_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_progress_history_source_account
    ON np_progress_history(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_user
    ON np_progress_history(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_content
    ON np_progress_history(content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_created
    ON np_progress_history(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_user_created
    ON np_progress_history(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS np_progress_watchlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    priority INTEGER DEFAULT 0,
    added_from VARCHAR(64),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, content_type, content_id)
);

CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_source_account
    ON np_progress_watchlists(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_user
    ON np_progress_watchlists(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_priority
    ON np_progress_watchlists(priority DESC);
CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_user_priority
    ON np_progress_watchlists(user_id, priority DESC);

CREATE TABLE IF NOT EXISTS np_progress_favorites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, content_type, content_id)
);

CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_source_account
    ON np_progress_favorites(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_user
    ON np_progress_favorites(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_created
    ON np_progress_favorites(created_at DESC);

CREATE TABLE IF NOT EXISTS np_progress_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) DEFAULT 'primary',
    event_type VARCHAR(128),
    payload JSONB,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_source_account
    ON np_progress_webhook_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_type
    ON np_progress_webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_processed
    ON np_progress_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_created
    ON np_progress_webhook_events(created_at DESC);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

// Ping verifies the database connection is alive.
func (d *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := d.pool.Exec(ctx, "SELECT 1")
	return err
}

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
func (d *DB) GetContinueWatching(userID string, limit int) ([]ContinueWatchingItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent, updated_at, metadata
		FROM np_progress_positions
		WHERE source_account_id = $1
			AND user_id = $2
			AND completed = FALSE
			AND progress_percent > 1
			AND progress_percent < $3
		ORDER BY updated_at DESC
		LIMIT $4`,
		d.sourceAccountID, userID, d.completeThreshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get continue watching: %w", err)
	}
	defer rows.Close()

	var items []ContinueWatchingItem
	for rows.Next() {
		var item ContinueWatchingItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.PositionSeconds, &item.DurationSeconds, &item.ProgressPercent, &item.UpdatedAt, &item.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scan continue watching: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []ContinueWatchingItem{}
	}
	return items, rows.Err()
}

// GetRecentlyWatched returns the most recently updated positions for a user.
func (d *DB) GetRecentlyWatched(userID string, limit int) ([]RecentlyWatchedItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent, completed,
			completed_at, updated_at, metadata
		FROM np_progress_positions
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
		LIMIT $3`,
		d.sourceAccountID, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recently watched: %w", err)
	}
	defer rows.Close()

	var items []RecentlyWatchedItem
	for rows.Next() {
		var item RecentlyWatchedItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.PositionSeconds, &item.DurationSeconds, &item.ProgressPercent, &item.Completed,
			&item.CompletedAt, &item.UpdatedAt, &item.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scan recently watched: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []RecentlyWatchedItem{}
	}
	return items, rows.Err()
}

// =========================================================================
// Progress History
// =========================================================================

// insertHistoryEvent records a history event directly (no sampling).
func (d *DB) insertHistoryEvent(userID, contentType, contentID, action string, positionSeconds *float64, deviceID *string, sessionID *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = d.pool.Exec(ctx,
		`INSERT INTO np_progress_history (
			source_account_id, user_id, content_type, content_id,
			action, position_seconds, device_id, session_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		d.sourceAccountID, userID, contentType, contentID, action, positionSeconds, deviceID, sessionID,
	)
}

// GetUserHistory returns history events for a user, ordered by created_at DESC.
func (d *DB) GetUserHistory(userID string, limit, offset int) ([]ProgressHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			action, position_seconds, device_id, session_id, created_at
		FROM np_progress_history
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get user history: %w", err)
	}
	defer rows.Close()

	var items []ProgressHistory
	for rows.Next() {
		var h ProgressHistory
		if err := rows.Scan(
			&h.ID, &h.SourceAccountID, &h.UserID, &h.ContentType, &h.ContentID,
			&h.Action, &h.PositionSeconds, &h.DeviceID, &h.SessionID, &h.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		items = append(items, h)
	}
	if items == nil {
		items = []ProgressHistory{}
	}
	return items, rows.Err()
}

// =========================================================================
// Watchlist
// =========================================================================

// AddToWatchlist inserts or updates a watchlist item (upsert on conflict).
func (d *DB) AddToWatchlist(req AddToWatchlistRequest) (*WatchlistItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	var item WatchlistItem
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_progress_watchlists (
			source_account_id, user_id, content_type, content_id,
			priority, added_from, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (source_account_id, user_id, content_type, content_id)
		DO UPDATE SET
			priority = EXCLUDED.priority,
			notes = EXCLUDED.notes
		RETURNING id, source_account_id, user_id, content_type, content_id,
			priority, added_from, notes, created_at`,
		d.sourceAccountID, req.UserID, req.ContentType, req.ContentID,
		priority, req.AddedFrom, req.Notes,
	).Scan(
		&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
		&item.Priority, &item.AddedFrom, &item.Notes, &item.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add to watchlist: %w", err)
	}
	return &item, nil
}

// GetWatchlist returns a user's watchlist ordered by priority DESC then created_at DESC.
func (d *DB) GetWatchlist(userID string, limit, offset int) ([]WatchlistItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			priority, added_from, notes, created_at
		FROM np_progress_watchlists
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY priority DESC, created_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get watchlist: %w", err)
	}
	defer rows.Close()

	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.Priority, &item.AddedFrom, &item.Notes, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan watchlist: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []WatchlistItem{}
	}
	return items, rows.Err()
}

// UpdateWatchlistItem updates priority and/or notes on a watchlist item.
func (d *DB) UpdateWatchlistItem(userID, contentType, contentID string, req UpdateWatchlistRequest) (*WatchlistItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setParts := []string{}
	args := []interface{}{d.sourceAccountID, userID, contentType, contentID}
	idx := 5

	if req.Priority != nil {
		setParts = append(setParts, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *req.Priority)
		idx++
	}
	if req.Notes != nil {
		setParts = append(setParts, fmt.Sprintf("notes = $%d", idx))
		args = append(args, *req.Notes)
		idx++
	}

	if len(setParts) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf(
		`UPDATE np_progress_watchlists
		SET %s
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4
		RETURNING id, source_account_id, user_id, content_type, content_id,
			priority, added_from, notes, created_at`,
		strings.Join(setParts, ", "),
	)

	var item WatchlistItem
	err := d.pool.QueryRow(ctx, query, args...).Scan(
		&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
		&item.Priority, &item.AddedFrom, &item.Notes, &item.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update watchlist item: %w", err)
	}
	return &item, nil
}

// RemoveFromWatchlist deletes a watchlist item. Returns true if a row was deleted.
func (d *DB) RemoveFromWatchlist(userID, contentType, contentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_progress_watchlists
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
		d.sourceAccountID, userID, contentType, contentID,
	)
	if err != nil {
		return false, fmt.Errorf("remove from watchlist: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// =========================================================================
// Favorites
// =========================================================================

// AddToFavorites inserts a favorite item. On conflict, returns the existing row.
func (d *DB) AddToFavorites(req AddToFavoritesRequest) (*FavoriteItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var item FavoriteItem
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_progress_favorites (
			source_account_id, user_id, content_type, content_id
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source_account_id, user_id, content_type, content_id)
		DO UPDATE SET source_account_id = EXCLUDED.source_account_id
		RETURNING id, source_account_id, user_id, content_type, content_id, created_at`,
		d.sourceAccountID, req.UserID, req.ContentType, req.ContentID,
	).Scan(
		&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add to favorites: %w", err)
	}
	return &item, nil
}

// GetFavorites returns a user's favorites ordered by created_at DESC.
func (d *DB) GetFavorites(userID string, limit, offset int) ([]FavoriteItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id, created_at
		FROM np_progress_favorites
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get favorites: %w", err)
	}
	defer rows.Close()

	var items []FavoriteItem
	for rows.Next() {
		var item FavoriteItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []FavoriteItem{}
	}
	return items, rows.Err()
}

// RemoveFromFavorites deletes a favorite item. Returns true if a row was deleted.
func (d *DB) RemoveFromFavorites(userID, contentType, contentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_progress_favorites
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
		d.sourceAccountID, userID, contentType, contentID,
	)
	if err != nil {
		return false, fmt.Errorf("remove from favorites: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// =========================================================================
// Webhook Events
// =========================================================================

// ListWebhookEvents returns webhook events, optionally filtered by event type.
func (d *DB) ListWebhookEvents(eventType string, limit, offset int) ([]WebhookEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rows pgx.Rows
	var err error

	if eventType != "" {
		rows, err = d.pool.Query(ctx,
			`SELECT id, source_account_id, event_type, payload, processed, processed_at, error, created_at
			FROM np_progress_webhook_events
			WHERE source_account_id = $1 AND event_type = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4`,
			d.sourceAccountID, eventType, limit, offset,
		)
	} else {
		rows, err = d.pool.Query(ctx,
			`SELECT id, source_account_id, event_type, payload, processed, processed_at, error, created_at
			FROM np_progress_webhook_events
			WHERE source_account_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`,
			d.sourceAccountID, limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list webhook events: %w", err)
	}
	defer rows.Close()

	var items []WebhookEvent
	for rows.Next() {
		var evt WebhookEvent
		if err := rows.Scan(
			&evt.ID, &evt.SourceAccountID, &evt.EventType, &evt.Payload,
			&evt.Processed, &evt.ProcessedAt, &evt.Error, &evt.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan webhook event: %w", err)
		}
		items = append(items, evt)
	}
	if items == nil {
		items = []WebhookEvent{}
	}
	return items, rows.Err()
}

// =========================================================================
// Statistics
// =========================================================================

// GetUserStats returns aggregated statistics for a single user.
func (d *DB) GetUserStats(userID string) (*UserStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var totalSeconds float64
	var completed, inProgress, watchlistCount, favoritesCount int64
	var mostWatchedType *string
	var recentActivity *time.Time

	err := d.pool.QueryRow(ctx,
		`WITH watch_time AS (
			SELECT COALESCE(SUM(position_seconds), 0) as total_seconds
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
		),
		counts AS (
			SELECT
				COUNT(*) FILTER (WHERE completed = TRUE) as completed,
				COUNT(*) FILTER (WHERE completed = FALSE AND progress_percent > 1) as in_progress
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
		),
		watchlist AS (
			SELECT COUNT(*) as count
			FROM np_progress_watchlists
			WHERE source_account_id = $1 AND user_id = $2
		),
		favorites AS (
			SELECT COUNT(*) as count
			FROM np_progress_favorites
			WHERE source_account_id = $1 AND user_id = $2
		),
		most_watched AS (
			SELECT content_type
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
			GROUP BY content_type
			ORDER BY COUNT(*) DESC
			LIMIT 1
		),
		recent AS (
			SELECT MAX(updated_at) as last_activity
			FROM np_progress_positions
			WHERE source_account_id = $1 AND user_id = $2
		)
		SELECT
			w.total_seconds,
			c.completed,
			c.in_progress,
			wl.count,
			f.count,
			mw.content_type,
			r.last_activity
		FROM watch_time w
		CROSS JOIN counts c
		CROSS JOIN watchlist wl
		CROSS JOIN favorites f
		LEFT JOIN most_watched mw ON TRUE
		LEFT JOIN recent r ON TRUE`,
		d.sourceAccountID, userID,
	).Scan(&totalSeconds, &completed, &inProgress, &watchlistCount, &favoritesCount, &mostWatchedType, &recentActivity)
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
	}

	return &UserStats{
		TotalWatchTimeSeconds: totalSeconds,
		TotalWatchTimeHours:   totalSeconds / 3600,
		ContentCompleted:      completed,
		ContentInProgress:     inProgress,
		WatchlistCount:        watchlistCount,
		FavoritesCount:        favoritesCount,
		MostWatchedType:       mostWatchedType,
		RecentActivity:        recentActivity,
	}, nil
}

// GetPluginStats returns aggregated plugin-wide statistics.
func (d *DB) GetPluginStats() (*PluginStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var totalUsers, totalPositions, totalCompleted, totalInProgress int64
	var totalWatchlist, totalFavorites, totalHistoryEvts int64
	var lastActivity *time.Time

	err := d.pool.QueryRow(ctx,
		`WITH users AS (
			SELECT COUNT(DISTINCT user_id) as count
			FROM np_progress_positions
			WHERE source_account_id = $1
		),
		positions AS (
			SELECT
				COUNT(*) as total,
				COUNT(*) FILTER (WHERE completed = TRUE) as completed,
				COUNT(*) FILTER (WHERE completed = FALSE AND progress_percent > 1) as in_progress,
				MAX(updated_at) as last_activity
			FROM np_progress_positions
			WHERE source_account_id = $1
		),
		watchlist AS (
			SELECT COUNT(*) as count
			FROM np_progress_watchlists
			WHERE source_account_id = $1
		),
		favorites AS (
			SELECT COUNT(*) as count
			FROM np_progress_favorites
			WHERE source_account_id = $1
		),
		history AS (
			SELECT COUNT(*) as count
			FROM np_progress_history
			WHERE source_account_id = $1
		)
		SELECT
			u.count,
			p.total,
			p.completed,
			p.in_progress,
			w.count,
			f.count,
			h.count,
			p.last_activity
		FROM users u
		CROSS JOIN positions p
		CROSS JOIN watchlist w
		CROSS JOIN favorites f
		CROSS JOIN history h`,
		d.sourceAccountID,
	).Scan(&totalUsers, &totalPositions, &totalCompleted, &totalInProgress,
		&totalWatchlist, &totalFavorites, &totalHistoryEvts, &lastActivity)
	if err != nil {
		return nil, fmt.Errorf("get plugin stats: %w", err)
	}

	return &PluginStats{
		TotalUsers:       totalUsers,
		TotalPositions:   totalPositions,
		TotalCompleted:   totalCompleted,
		TotalInProgress:  totalInProgress,
		TotalWatchlist:   totalWatchlist,
		TotalFavorites:   totalFavorites,
		TotalHistoryEvts: totalHistoryEvts,
		LastActivity:     lastActivity,
	}, nil
}

// =========================================================================
// Helpers
// =========================================================================

// scanPositions scans rows into a slice of ProgressPosition.
func scanPositions(rows pgx.Rows) ([]ProgressPosition, error) {
	var positions []ProgressPosition
	for rows.Next() {
		var pos ProgressPosition
		if err := rows.Scan(
			&pos.ID, &pos.SourceAccountID, &pos.UserID, &pos.ContentType, &pos.ContentID,
			&pos.PositionSeconds, &pos.DurationSeconds, &pos.ProgressPercent,
			&pos.Completed, &pos.CompletedAt, &pos.DeviceID, &pos.AudioTrack, &pos.SubtitleTrack,
			&pos.Quality, &pos.Metadata, &pos.UpdatedAt, &pos.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, pos)
	}
	if positions == nil {
		positions = []ProgressPosition{}
	}
	return positions, rows.Err()
}
