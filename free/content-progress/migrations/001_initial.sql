-- content-progress plugin: initial schema
-- CODE WINS: table names from internal/db.go (np_progress_* prefix, not np_content_*)
-- 5 tables: np_progress_positions, np_progress_history, np_progress_watchlists,
--           np_progress_favorites, np_progress_webhook_events

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_progress_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    position_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_seconds DOUBLE PRECISION,
    progress_percent DOUBLE PRECISION DEFAULT 0,
    completed BOOLEAN DEFAULT FALSE,
    completed_at TIMESTAMPTZ,
    device_id VARCHAR(255),
    audio_track VARCHAR(16),
    subtitle_track VARCHAR(16),
    quality VARCHAR(16),
    metadata JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, content_type, content_id)
);

CREATE INDEX IF NOT EXISTS idx_np_progress_positions_source_account ON np_progress_positions(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_user ON np_progress_positions(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_content ON np_progress_positions(content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_completed ON np_progress_positions(completed);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_updated ON np_progress_positions(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_progress_positions_user_updated ON np_progress_positions(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS np_progress_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    action VARCHAR(16) NOT NULL DEFAULT 'play',
    position_seconds DOUBLE PRECISION,
    device_id VARCHAR(255),
    session_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_progress_history_source_account ON np_progress_history(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_user ON np_progress_history(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_content ON np_progress_history(content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_created ON np_progress_history(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_progress_history_user_created ON np_progress_history(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS np_progress_watchlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    priority INTEGER DEFAULT 0,
    added_from VARCHAR(64),
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, content_type, content_id)
);

CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_source_account ON np_progress_watchlists(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_user ON np_progress_watchlists(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_priority ON np_progress_watchlists(priority DESC);
CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_user_priority ON np_progress_watchlists(user_id, priority DESC);

CREATE TABLE IF NOT EXISTS np_progress_favorites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, content_type, content_id)
);

CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_source_account ON np_progress_favorites(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_user ON np_progress_favorites(user_id);
CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_created ON np_progress_favorites(created_at DESC);

CREATE TABLE IF NOT EXISTS np_progress_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_type VARCHAR(128),
    payload JSONB,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_source_account ON np_progress_webhook_events(source_account_id);
