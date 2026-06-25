package internal

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

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
// Size-cap exception: SQL DDL migration — 127L of linear SQL statements; splitting across files adds no value and breaks transactional migration semantics.
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

