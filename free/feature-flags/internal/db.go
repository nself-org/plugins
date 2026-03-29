package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS np_feature_flags_flags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    description TEXT,
    enabled BOOLEAN DEFAULT false,
    default_value JSONB DEFAULT 'false',
    rules JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_key ON np_feature_flags_flags(key);
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_enabled ON np_feature_flags_flags(enabled);

CREATE TABLE IF NOT EXISTS np_feature_flags_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    rules JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_ff_segments_name ON np_feature_flags_segments(name);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

// --- Flag types ---

// Flag represents a feature flag row.
type Flag struct {
	ID           string          `json:"id"`
	Key          string          `json:"key"`
	Name         *string         `json:"name"`
	Description  *string         `json:"description"`
	Enabled      bool            `json:"enabled"`
	DefaultValue json.RawMessage `json:"default_value"`
	Rules        json.RawMessage `json:"rules"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// CreateFlagRequest is the JSON body for creating a flag.
type CreateFlagRequest struct {
	Key          string           `json:"key"`
	Name         *string          `json:"name,omitempty"`
	Description  *string          `json:"description,omitempty"`
	Enabled      *bool            `json:"enabled,omitempty"`
	DefaultValue *json.RawMessage `json:"default_value,omitempty"`
	Rules        *json.RawMessage `json:"rules,omitempty"`
}

// UpdateFlagRequest is the JSON body for updating a flag.
type UpdateFlagRequest struct {
	Name         *string          `json:"name,omitempty"`
	Description  *string          `json:"description,omitempty"`
	Enabled      *bool            `json:"enabled,omitempty"`
	DefaultValue *json.RawMessage `json:"default_value,omitempty"`
	Rules        *json.RawMessage `json:"rules,omitempty"`
}

// --- Segment types ---

// Segment represents a user segment row.
type Segment struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Rules     json.RawMessage `json:"rules"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// --- Flag CRUD ---

// CreateFlag inserts a new feature flag.
func (d *DB) CreateFlag(req CreateFlagRequest) (*Flag, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	defaultVal := json.RawMessage(`false`)
	if req.DefaultValue != nil {
		defaultVal = *req.DefaultValue
	}
	rules := json.RawMessage(`[]`)
	if req.Rules != nil {
		rules = *req.Rules
	}

	var f Flag
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_feature_flags_flags (key, name, description, enabled, default_value, rules)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, key, name, description, enabled, default_value, rules, created_at, updated_at`,
		req.Key, req.Name, req.Description, enabled, defaultVal, rules,
	).Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.Enabled, &f.DefaultValue, &f.Rules, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create flag: %w", err)
	}
	return &f, nil
}

// ListFlags returns all feature flags ordered by creation time.
func (d *DB) ListFlags() ([]Flag, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, key, name, description, enabled, default_value, rules, created_at, updated_at
		 FROM np_feature_flags_flags ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list flags: %w", err)
	}
	defer rows.Close()

	var flags []Flag
	for rows.Next() {
		var f Flag
		if err := rows.Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.Enabled, &f.DefaultValue, &f.Rules, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan flag: %w", err)
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

// GetFlag returns a single flag by key.
func (d *DB) GetFlag(key string) (*Flag, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var f Flag
	err := d.pool.QueryRow(ctx,
		`SELECT id, key, name, description, enabled, default_value, rules, created_at, updated_at
		 FROM np_feature_flags_flags WHERE key = $1`, key,
	).Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.Enabled, &f.DefaultValue, &f.Rules, &f.CreatedAt, &f.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get flag: %w", err)
	}
	return &f, nil
}

// UpdateFlag updates an existing flag by key.
func (d *DB) UpdateFlag(key string, req UpdateFlagRequest) (*Flag, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build dynamic SET clause
	sets := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *req.Description)
		idx++
	}
	if req.Enabled != nil {
		sets = append(sets, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}
	if req.DefaultValue != nil {
		sets = append(sets, fmt.Sprintf("default_value = $%d", idx))
		args = append(args, *req.DefaultValue)
		idx++
	}
	if req.Rules != nil {
		sets = append(sets, fmt.Sprintf("rules = $%d", idx))
		args = append(args, *req.Rules)
		idx++
	}

	if len(sets) == 1 {
		// Nothing to update besides updated_at
		return d.GetFlag(key)
	}

	args = append(args, key)
	query := fmt.Sprintf(
		`UPDATE np_feature_flags_flags SET %s WHERE key = $%d
		 RETURNING id, key, name, description, enabled, default_value, rules, created_at, updated_at`,
		joinStrings(sets, ", "), idx,
	)

	var f Flag
	err := d.pool.QueryRow(ctx, query, args...).Scan(
		&f.ID, &f.Key, &f.Name, &f.Description, &f.Enabled, &f.DefaultValue, &f.Rules, &f.CreatedAt, &f.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update flag: %w", err)
	}
	return &f, nil
}

// DeleteFlag removes a flag by key. Returns true if a row was deleted.
func (d *DB) DeleteFlag(key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_feature_flags_flags WHERE key = $1`, key)
	if err != nil {
		return false, fmt.Errorf("delete flag: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// --- Segment CRUD ---

// GetSegment returns a segment by ID.
func (d *DB) GetSegment(id string) (*Segment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var s Segment
	err := d.pool.QueryRow(ctx,
		`SELECT id, name, rules, created_at, updated_at
		 FROM np_feature_flags_segments WHERE id = $1`, id,
	).Scan(&s.ID, &s.Name, &s.Rules, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment: %w", err)
	}
	return &s, nil
}

// joinStrings joins a slice of strings with a separator (avoids importing strings).
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}
