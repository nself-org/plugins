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
	ID          string          `json:"id"`
	DeviceToken string          `json:"device_token"`
	Platform    string          `json:"platform"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	LastError   *string         `json:"last_error,omitempty"`
	DedupeHash  string          `json:"dedupe_hash"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
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
