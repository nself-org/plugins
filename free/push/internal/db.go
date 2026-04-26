package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxStatus constants for np_push_outbox.status.
const (
	StatusPending   = "pending"
	StatusQueued    = "queued"
	StatusDelivered = "delivered"
	StatusRetrying  = "retrying"
	StatusFailed    = "failed"
)

// OutboxRow represents a row in np_push_outbox.
type OutboxRow struct {
	ID           string          `json:"id"`
	DeviceToken  string          `json:"device_token"`
	Platform     string          `json:"platform"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"`
	Attempts     int             `json:"attempts"`
	LastError    *string         `json:"last_error,omitempty"`
	DedupeHash   string          `json:"dedupe_hash"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Device represents a registered device in np_push_devices.
type Device struct {
	ID          string    `json:"id"`
	DeviceToken string    `json:"device_token"`
	Platform    string    `json:"platform"`
	AppID       string    `json:"app_id"`
	UserID      *string   `json:"user_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Migrate runs idempotent schema creation for np_push_outbox and np_push_devices.
// All statements use IF NOT EXISTS so re-running on an existing schema is safe.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queries := []string{
		// Device registry: maps device tokens to users/apps.
		// Tokens are stored in plain text — they are not secrets (APNs/FCM tokens
		// are semi-public identifiers; the real secrets are the provider credentials
		// held in env vars, never in the DB).
		`CREATE TABLE IF NOT EXISTS np_push_devices (
			id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			device_token TEXT        NOT NULL,
			platform     TEXT        NOT NULL CHECK (platform IN ('ios', 'android')),
			app_id       TEXT        NOT NULL DEFAULT 'default',
			user_id      TEXT,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// Unique constraint: one token per (platform, app_id) combination.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_np_push_devices_token_platform_app
			ON np_push_devices(device_token, platform, app_id)`,
		`CREATE INDEX IF NOT EXISTS idx_np_push_devices_user_id
			ON np_push_devices(user_id) WHERE user_id IS NOT NULL`,

		// Outbox: queued push notifications with delivery tracking.
		// dedupe_hash is SHA-256(device_token || platform || payload) to prevent
		// duplicate delivery on retry storms or at-least-once event trigger re-fires.
		`CREATE TABLE IF NOT EXISTS np_push_outbox (
			id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			device_token TEXT        NOT NULL,
			platform     TEXT        NOT NULL CHECK (platform IN ('ios', 'android')),
			payload      JSONB       NOT NULL,
			status       TEXT        NOT NULL DEFAULT 'pending'
			             CHECK (status IN ('pending','queued','delivered','retrying','failed')),
			attempts     INTEGER     NOT NULL DEFAULT 0,
			last_error   TEXT,
			dedupe_hash  TEXT        NOT NULL,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// Idempotency: unique on dedupe_hash prevents double-sends from retry bursts.
		// The hash incorporates device_token + platform + payload so distinct messages
		// to the same device are allowed; only exact-duplicate payloads are deduplicated.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_np_push_outbox_dedupe
			ON np_push_outbox(dedupe_hash)
			WHERE status != 'failed'`,
		`CREATE INDEX IF NOT EXISTS idx_np_push_outbox_status
			ON np_push_outbox(status)`,
		`CREATE INDEX IF NOT EXISTS idx_np_push_outbox_created_at
			ON np_push_outbox(created_at)`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("push migrate: %w", err)
		}
	}
	return nil
}

// DedupeHash returns SHA-256(deviceToken + "|" + platform + "|" + payload).
// Used as the idempotency key for np_push_outbox rows.
func DedupeHash(deviceToken, platform string, payload json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(deviceToken))
	h.Write([]byte("|"))
	h.Write([]byte(platform))
	h.Write([]byte("|"))
	h.Write(payload)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// InsertOutbox inserts a new outbox row, ignoring conflicts on dedupe_hash
// (idempotent — safe to call from Hasura event triggers that may fire more than once).
// Returns (rowID, alreadyExists, error).
func InsertOutbox(ctx context.Context, pool *pgxpool.Pool, deviceToken, platform string, payload json.RawMessage) (string, bool, error) {
	hash := DedupeHash(deviceToken, platform, payload)
	id := uuid.New().String()

	tag, err := pool.Exec(ctx,
		`INSERT INTO np_push_outbox (id, device_token, platform, payload, dedupe_hash)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (dedupe_hash) WHERE status != 'failed' DO NOTHING`,
		id, deviceToken, platform, payload, hash,
	)
	if err != nil {
		return "", false, fmt.Errorf("insert outbox: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Row already existed (duplicate event trigger or retry).
		return "", true, nil
	}
	return id, false, nil
}

// UpdateOutboxStatus updates the status, attempts, and last_error on an outbox row.
func UpdateOutboxStatus(ctx context.Context, pool *pgxpool.Pool, id, status string, attempts int, lastErr *string) error {
	_, err := pool.Exec(ctx,
		`UPDATE np_push_outbox
		    SET status = $2, attempts = $3, last_error = $4, updated_at = NOW()
		  WHERE id = $1`,
		id, status, attempts, lastErr,
	)
	if err != nil {
		return fmt.Errorf("update outbox status: %w", err)
	}
	return nil
}

// GetOutboxByID fetches a single outbox row by primary key.
func GetOutboxByID(ctx context.Context, pool *pgxpool.Pool, id string) (*OutboxRow, error) {
	row := &OutboxRow{}
	err := pool.QueryRow(ctx,
		`SELECT id, device_token, platform, payload, status, attempts, last_error, dedupe_hash, created_at, updated_at
		   FROM np_push_outbox WHERE id = $1`,
		id,
	).Scan(
		&row.ID, &row.DeviceToken, &row.Platform, &row.Payload,
		&row.Status, &row.Attempts, &row.LastError, &row.DedupeHash,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get outbox row: %w", err)
	}
	return row, nil
}

// UpsertDevice registers or updates a device token.
func UpsertDevice(ctx context.Context, pool *pgxpool.Pool, token, platform, appID string, userID *string) (*Device, error) {
	d := &Device{}
	err := pool.QueryRow(ctx,
		`INSERT INTO np_push_devices (device_token, platform, app_id, user_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (device_token, platform, app_id)
		 DO UPDATE SET user_id = EXCLUDED.user_id, updated_at = NOW()
		 RETURNING id, device_token, platform, app_id, user_id, created_at, updated_at`,
		token, platform, appID, userID,
	).Scan(&d.ID, &d.DeviceToken, &d.Platform, &d.AppID, &d.UserID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert device: %w", err)
	}
	return d, nil
}
