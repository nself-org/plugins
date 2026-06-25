package internal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with feature-flags table operations.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// InitSchema creates tables and indexes if they do not exist.
// This covers migration 001 (initial schema). Migration 002 (audit table) lives in
// migrations/002_add_audit.sql and is applied by the CLI migration runner.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS np_feature_flags_flags (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    key             VARCHAR(255) NOT NULL UNIQUE,
    name            VARCHAR(255),
    description     TEXT,
    type            VARCHAR(50)  NOT NULL DEFAULT 'boolean'
                    CHECK (type IN ('boolean', 'percentage', 'kill_switch', 'experiment', 'config')),
    enabled         BOOLEAN      DEFAULT false,
    rollout_pct     INTEGER      CHECK (rollout_pct IS NULL OR (rollout_pct >= 0 AND rollout_pct <= 100)),
    stale_after_days INTEGER     CHECK (stale_after_days IS NULL OR stale_after_days > 0),
    default_value   JSONB        DEFAULT 'false',
    rules           JSONB        DEFAULT '[]',
    created_at      TIMESTAMPTZ  DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_key ON np_feature_flags_flags(key);
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_enabled ON np_feature_flags_flags(enabled);
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_type ON np_feature_flags_flags(type);

CREATE TABLE IF NOT EXISTS np_feature_flags_segments (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL UNIQUE,
    rules      JSONB        DEFAULT '[]',
    created_at TIMESTAMPTZ  DEFAULT NOW(),
    updated_at TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_ff_segments_name ON np_feature_flags_segments(name);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

// --- Flag types ---

// Flag represents a feature flag row.
type Flag struct {
	ID              string          `json:"id"`
	Key             string          `json:"key"`
	Name            *string         `json:"name"`
	Description     *string         `json:"description"`
	Type            string          `json:"type"`
	Enabled         bool            `json:"enabled"`
	RolloutPct      *int            `json:"rollout_pct"`
	StaleAfterDays  *int            `json:"stale_after_days"`
	DefaultValue    json.RawMessage `json:"default_value"`
	Rules           json.RawMessage `json:"rules"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// flagColumns is the SELECT column list used in all flag queries.
const flagColumns = `id, key, name, description, type, enabled, rollout_pct, stale_after_days, default_value, rules, created_at, updated_at`

// scanFlag scans a row into a Flag struct (column order matches flagColumns).
