// Package store — operation persistence for /sync/push.
//
// Operations (event envelopes) are written to np_sync_events with the user_id
// taken from the JWT — never trusting the client-supplied field. The insert is
// idempotent: re-pushing the same (event_id, user_id) tuple returns an
// "already-applied" result rather than a duplicate row or an error.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PushEvent is the server-side normalized form of one event envelope. JSON
// shape mirrors what the Rust nclaw core (`sync::lww::EventEnvelope`) emits
// after V04-F02 (user_id baked into signing material) and V04-F03 (canonical
// JSON serialization).
type PushEvent struct {
	EventID       string          `json:"event_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Op            string          `json:"op"`
	HLCWallMs     int64           `json:"hlc_wall_ms"`
	HLCLamport    int64           `json:"hlc_lamport"`
	HLCDeviceID   string          `json:"hlc_device_id"`
	UserID        string          `json:"user_id"`
	DeviceID      string          `json:"device_id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Signature     []byte          `json:"signature"`
}

// ErrInvalidOp is returned when op is not insert|update|delete.
var ErrInvalidOp = errors.New("invalid op: must be insert, update, or delete")

// ErrDevicePubkeyNotFound is returned when no active device row exists for the
// (device_id, user_id) tuple — signature verification cannot proceed.
var ErrDevicePubkeyNotFound = errors.New("device public key not found or device revoked")

// OperationStore persists pushed event envelopes. Implemented by *Store
// (Postgres) in production and MemOperationStore in tests.
type OperationStore interface {
	// DevicePubkey returns the active device's Ed25519 public key, used by the
	// caller to verify the operation's signature. Returns ErrDevicePubkeyNotFound
	// when the device does not exist, is revoked, or belongs to a different user.
	DevicePubkey(ctx context.Context, userID, deviceID string) ([]byte, error)

	// Insert persists one PushEvent. Idempotent on (event_id, user_id): a second
	// call with the same tuple is a no-op and returns (false, nil) — false means
	// "row already existed, no insert performed". A genuine insert returns
	// (true, nil). On constraint failure or transport error, returns (_, err).
	Insert(ctx context.Context, ev PushEvent) (inserted bool, err error)
}

// ValidateOp returns nil iff op is one of the three allowed verbs.
// Exported so handlers can validate before constructing a PushEvent.
func ValidateOp(op string) error {
	switch op {
	case "insert", "update", "delete":
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidOp, op)
}

// ---------- Postgres implementation ----------

// DevicePubkey implements OperationStore for *Store.
func (s *Store) DevicePubkey(ctx context.Context, userID, deviceID string) ([]byte, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("sync/store: pool unavailable")
	}
	const q = `
		SELECT device_pubkey
		  FROM np_devices
		 WHERE id = $1::uuid
		   AND user_id = $2::uuid
		   AND revoked_at IS NULL`
	var pub []byte
	err := s.pool.QueryRow(ctx, q, deviceID, userID).Scan(&pub)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDevicePubkeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sync/store: device pubkey lookup: %w", err)
	}
	return pub, nil
}

// Insert implements OperationStore for *Store.
//
// Uses INSERT ... ON CONFLICT (event_id) DO NOTHING and RETURNING xmax = 0 to
// distinguish a genuine insert from an idempotent re-push. xmax = 0 on a row
// returned by RETURNING means the row was newly inserted; non-zero means the
// row pre-existed (no insert happened, the existing row is returned via
// DO NOTHING semantics — but since DO NOTHING does not return the conflicting
// row, we instead check rowsAffected.
func (s *Store) Insert(ctx context.Context, ev PushEvent) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("sync/store: pool unavailable")
	}
	if err := ValidateOp(ev.Op); err != nil {
		return false, err
	}
	var tenant any
	if ev.TenantID != "" {
		tenant = ev.TenantID
	} else {
		tenant = nil
	}
	var payload any
	if len(ev.Payload) > 0 {
		payload = []byte(ev.Payload)
	} else {
		payload = nil
	}
	const q = `
		INSERT INTO np_sync_events (
			event_id, user_id, tenant_id, device_id,
			entity_type, entity_id, op,
			hlc_wall_ms, hlc_lamport,
			payload, schema_version, signature
		)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid,
			$5, $6::uuid, $7,
			$8, $9,
			$10, $11, $12)
		ON CONFLICT (event_id) DO NOTHING`
	tag, err := s.pool.Exec(ctx, q,
		ev.EventID, ev.UserID, tenant, ev.DeviceID,
		ev.EntityType, ev.EntityID, ev.Op,
		ev.HLCWallMs, ev.HLCLamport,
		payload, ev.SchemaVersion, ev.Signature,
	)
	if err != nil {
		return false, fmt.Errorf("sync/store: insert event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Ensure *Store satisfies OperationStore at compile time.
var _ OperationStore = (*Store)(nil)
