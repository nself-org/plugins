package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgx connection pool with vault-specific data methods.
type DB struct {
	pool *pgxpool.Pool
}

// New opens a pgx pool against the given DATABASE_URL.
func New(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("vault/store: parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("vault/store: connect: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Ping checks the DB connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Close releases pool connections.
func (db *DB) Close() {
	db.pool.Close()
}

// RunMigration applies the embedded SQL migration (001_vault_schema).
// Idempotent — uses IF NOT EXISTS throughout.
func (db *DB) RunMigration(ctx context.Context, sql string) error {
	_, err := db.pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("vault/store: migration: %w", err)
	}
	return nil
}

// ---------- Device ----------

// Device represents a registered end-user device.
type Device struct {
	ID          string
	UserID      string
	TenantID    string
	DevicePubkey []byte
	DeviceLabel string
	Platform    string
	LastSeenAt  time.Time
	RevokedAt   *time.Time
}

// CreateDevice inserts a new device row and returns its UUID.
func (db *DB) CreateDevice(ctx context.Context, userID, tenantID string, pubkey []byte, label, platform string) (string, error) {
	var id string
	err := db.pool.QueryRow(ctx, `
		INSERT INTO np_vault_devices (user_id, tenant_id, device_pubkey, device_label, platform)
		VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5)
		RETURNING id::text
	`, userID, tenantID, pubkey, label, platform).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("vault/store: create device: %w", err)
	}
	return id, nil
}

// ListDevices returns all non-revoked devices for a user.
func (db *DB) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id::text, user_id::text, COALESCE(tenant_id::text,''), device_pubkey,
		       COALESCE(device_label,''), COALESCE(platform,''), last_seen_at
		FROM np_vault_devices
		WHERE user_id = $1::uuid AND revoked_at IS NULL
		ORDER BY last_seen_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("vault/store: list devices: %w", err)
	}
	defer rows.Close()
	return scanDevices(rows)
}

// DeviceBelongsToUser reports whether the given deviceID is owned by userID and not revoked.
// Returns ErrNotFound when the device does not exist, belongs to a different user, or
// has been revoked. This is the authoritative ownership check used by handler-layer
// guards to prevent V05-F4 (cross-user envelope access via known device UUID).
func (db *DB) DeviceBelongsToUser(ctx context.Context, deviceID, userID string) error {
	var owner string
	err := db.pool.QueryRow(ctx, `
		SELECT user_id::text FROM np_vault_devices
		WHERE id = $1::uuid AND revoked_at IS NULL
	`, deviceID).Scan(&owner)
	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("vault/store: device ownership check: %w", err)
	}
	if owner != userID {
		// Treat foreign-device access as not-found to avoid leaking existence.
		return ErrNotFound
	}
	return nil
}

// RecordBelongsToUser reports whether the given recordID is owned by userID and not revoked.
// Returns ErrNotFound when the record is missing, foreign-owned, or revoked.
// Used as a defense-in-depth guard alongside device ownership checks.
func (db *DB) RecordBelongsToUser(ctx context.Context, recordID, userID string) error {
	var owner string
	err := db.pool.QueryRow(ctx, `
		SELECT user_id::text FROM np_vault_records
		WHERE id = $1::uuid AND revoked_at IS NULL
	`, recordID).Scan(&owner)
	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("vault/store: record ownership check: %w", err)
	}
	if owner != userID {
		return ErrNotFound
	}
	return nil
}

// RevokeDevice sets revoked_at on a device (soft delete).
// Returns ErrNotFound if the device does not belong to the user.
func (db *DB) RevokeDevice(ctx context.Context, deviceID, userID string) error {
	tag, err := db.pool.Exec(ctx, `
		UPDATE np_vault_devices SET revoked_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL
	`, deviceID, userID)
	if err != nil {
		return fmt.Errorf("vault/store: revoke device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanDevices(rows pgx.Rows) ([]Device, error) {
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.TenantID, &d.DevicePubkey,
			&d.DeviceLabel, &d.Platform, &d.LastSeenAt); err != nil {
			return nil, fmt.Errorf("vault/store: scan device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------- Record ----------

// Record is a logical credential entry (without ciphertext — ciphertext lives in envelopes).
type Record struct {
	ID             string
	UserID         string
	TenantID       string
	CredentialKind string
	Label          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	RevokedAt      *time.Time
}

// CreateRecord inserts a new record and returns its UUID.
func (db *DB) CreateRecord(ctx context.Context, userID, tenantID, kind, label string) (string, error) {
	var id string
	err := db.pool.QueryRow(ctx, `
		INSERT INTO np_vault_records (user_id, tenant_id, credential_kind, label)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4)
		RETURNING id::text
	`, userID, tenantID, kind, label).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("vault/store: create record: %w", err)
	}
	return id, nil
}

// ListRecords returns non-revoked records visible to a user, with optional since cursor.
func (db *DB) ListRecords(ctx context.Context, userID string, since *time.Time) ([]Record, error) {
	q := `
		SELECT id::text, user_id::text, COALESCE(tenant_id::text,''),
		       credential_kind, label, created_at, updated_at
		FROM np_vault_records
		WHERE user_id = $1::uuid AND revoked_at IS NULL`
	args := []interface{}{userID}
	if since != nil {
		q += " AND created_at > $2"
		args = append(args, *since)
	}
	q += " ORDER BY created_at ASC"
	rows, err := db.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("vault/store: list records: %w", err)
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.UserID, &r.TenantID, &r.CredentialKind, &r.Label,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("vault/store: scan record: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RevokeRecord soft-deletes a record (cascade wipes envelopes via FK ON DELETE CASCADE).
func (db *DB) RevokeRecord(ctx context.Context, recordID, userID string) error {
	tag, err := db.pool.Exec(ctx, `
		UPDATE np_vault_records SET revoked_at = now(), updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL
	`, recordID, userID)
	if err != nil {
		return fmt.Errorf("vault/store: revoke record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Envelope ----------

// Envelope holds the per-device ciphertext for a record.
type Envelope struct {
	ID                 string
	RecordID           string
	DeviceID           string
	EnvelopeCiphertext []byte
	EnvelopeNonce      []byte
	SealedAt           time.Time
}

// CreateEnvelope stores or replaces the envelope for a (record, device) pair.
func (db *DB) CreateEnvelope(ctx context.Context, recordID, deviceID string, ciphertext, nonce []byte) (string, error) {
	var id string
	err := db.pool.QueryRow(ctx, `
		INSERT INTO np_vault_envelopes (record_id, device_id, envelope_ciphertext, envelope_nonce)
		VALUES ($1::uuid, $2::uuid, $3, $4)
		ON CONFLICT (record_id, device_id) DO UPDATE
		  SET envelope_ciphertext = EXCLUDED.envelope_ciphertext,
		      envelope_nonce      = EXCLUDED.envelope_nonce,
		      sealed_at           = now()
		RETURNING id::text
	`, recordID, deviceID, ciphertext, nonce).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("vault/store: create envelope: %w", err)
	}
	return id, nil
}

// GetEnvelope returns the envelope for the given (recordID, deviceID) pair.
func (db *DB) GetEnvelope(ctx context.Context, recordID, deviceID string) (*Envelope, error) {
	var e Envelope
	err := db.pool.QueryRow(ctx, `
		SELECT id::text, record_id::text, device_id::text,
		       envelope_ciphertext, envelope_nonce, sealed_at
		FROM np_vault_envelopes
		WHERE record_id = $1::uuid AND device_id = $2::uuid
	`, recordID, deviceID).Scan(
		&e.ID, &e.RecordID, &e.DeviceID,
		&e.EnvelopeCiphertext, &e.EnvelopeNonce, &e.SealedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("vault/store: get envelope: %w", err)
	}
	return &e, nil
}

// ---------- Audit ----------

// WriteAudit inserts an immutable audit log entry.
func (db *DB) WriteAudit(ctx context.Context, userID, tenantID, action string, recordID, deviceID *string, meta map[string]interface{}) error {
	_, err := db.pool.Exec(ctx, `
		INSERT INTO np_vault_audit (user_id, tenant_id, record_id, device_id, action, metadata)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3::uuid, $4::uuid, $5, $6)
	`, userID, tenantID,
		nullableUUID(recordID),
		nullableUUID(deviceID),
		action,
		jsonbArg(meta),
	)
	if err != nil {
		return fmt.Errorf("vault/store: write audit: %w", err)
	}
	return nil
}

// ---------- helpers ----------

// ErrNotFound signals a missing or unauthorized row.
var ErrNotFound = fmt.Errorf("vault/store: not found")

func nullableUUID(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func jsonbArg(m map[string]interface{}) interface{} {
	if m == nil {
		return "{}"
	}
	// pgx handles map[string]interface{} as JSONB natively.
	return m
}
