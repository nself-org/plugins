package internal

import (
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

// --- Flag types ---

// Flag represents a feature flag row.
type Flag struct {
	ID             string          `json:"id"`
	Key            string          `json:"key"`
	Name           *string         `json:"name"`
	Description    *string         `json:"description"`
	Type           string          `json:"type"`
	Enabled        bool            `json:"enabled"`
	RolloutPct     *int            `json:"rollout_pct"`
	StaleAfterDays *int            `json:"stale_after_days"`
	DefaultValue   json.RawMessage `json:"default_value"`
	Rules          json.RawMessage `json:"rules"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// flagColumns is the SELECT column list used in all flag queries.
const flagColumns = `id, key, name, description, type, enabled, rollout_pct, stale_after_days, default_value, rules, created_at, updated_at`

// scanFlag scans a row into a Flag struct (column order matches flagColumns).
