// cursors.go — per-device cursor advancement for the /ack endpoint.
//
// Cursors are HLC pairs (wall_ms, lamport) persisted in np_sync_cursors and
// keyed by device_id (FK to np_devices). Advance is monotonic — the stored
// cursor is the lexicographic max of the existing value and the requested
// value. Calling Advance with a cursor <= the stored value is a no-op and
// returns the stored value unchanged (idempotent).
package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
)

// Cursor is the HLC pair stored per device.
type Cursor struct {
	WallMs  int64 `json:"hlc_wall_ms"`
	Lamport int64 `json:"hlc_lamport"`
}

// Less reports whether c is lexicographically before other.
func (c Cursor) Less(other Cursor) bool {
	if c.WallMs != other.WallMs {
		return c.WallMs < other.WallMs
	}
	return c.Lamport < other.Lamport
}

// ErrDeviceNotFound is returned when a cursor is requested for a device that
// is absent from np_devices or has been revoked.
var ErrDeviceNotFound = errors.New("device not found")

// ErrDeviceUserMismatch is returned when the requesting user_id does not own
// the device_id being acked. Defense-in-depth on top of JWT verification.
var ErrDeviceUserMismatch = errors.New("device does not belong to user")

// CursorStore is the persistence boundary the /ack handler depends on.
// Production: *Store implements this. Tests use *MemCursorStore.
type CursorStore interface {
	// AdvanceCursor UPSERTs the cursor for (userID, deviceID), keeping the
	// lexicographic max of the requested and existing values. It enforces
	// that the device belongs to userID before any write. Returns the
	// stored cursor after the operation.
	AdvanceCursor(ctx context.Context, userID, deviceID string, requested Cursor) (Cursor, error)
}

// AdvanceCursor advances the device's stored cursor monotonically.
//
// The op runs in a single transaction:
//  1. Verify the device row exists, is not revoked, and belongs to user_id.
//  2. UPSERT np_sync_cursors so cursor never regresses (idempotent: re-acking
//     the same cursor is a no-op).
//
// Returns the cursor value AFTER the upsert (which may equal the existing
// stored value if requested was <= existing).
func (s *Store) AdvanceCursor(ctx context.Context, userID, deviceID string, requested Cursor) (Cursor, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Cursor{}, fmt.Errorf("sync/store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Device ownership + liveness check. revoked_at filter prevents acking
	// from a key that has been retired.
	var ownerID string
	err = tx.QueryRow(ctx,
		`SELECT user_id::text FROM np_devices WHERE id = $1 AND revoked_at IS NULL`,
		deviceID,
	).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cursor{}, ErrDeviceNotFound
	}
	if err != nil {
		return Cursor{}, fmt.Errorf("sync/store: device lookup: %w", err)
	}
	if ownerID != userID {
		return Cursor{}, ErrDeviceUserMismatch
	}

	// Monotonic UPSERT. CASE preserves the (wall_ms, lamport) PAIR as a unit
	// so we never mix existing.wall_ms with requested.lamport.
	var stored Cursor
	err = tx.QueryRow(ctx, `
        INSERT INTO np_sync_cursors (device_id, last_hlc_wall_ms, last_hlc_lamport, updated_at)
        VALUES ($1, $2, $3, now())
        ON CONFLICT (device_id) DO UPDATE
        SET last_hlc_wall_ms = CASE
                WHEN np_sync_cursors.last_hlc_wall_ms > EXCLUDED.last_hlc_wall_ms THEN np_sync_cursors.last_hlc_wall_ms
                WHEN np_sync_cursors.last_hlc_wall_ms < EXCLUDED.last_hlc_wall_ms THEN EXCLUDED.last_hlc_wall_ms
                ELSE np_sync_cursors.last_hlc_wall_ms
            END,
            last_hlc_lamport = CASE
                WHEN np_sync_cursors.last_hlc_wall_ms > EXCLUDED.last_hlc_wall_ms THEN np_sync_cursors.last_hlc_lamport
                WHEN np_sync_cursors.last_hlc_wall_ms < EXCLUDED.last_hlc_wall_ms THEN EXCLUDED.last_hlc_lamport
                ELSE GREATEST(np_sync_cursors.last_hlc_lamport, EXCLUDED.last_hlc_lamport)
            END,
            updated_at = now()
        RETURNING last_hlc_wall_ms, last_hlc_lamport
    `, deviceID, requested.WallMs, requested.Lamport).Scan(&stored.WallMs, &stored.Lamport)
	if err != nil {
		return Cursor{}, fmt.Errorf("sync/store: cursor upsert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Cursor{}, fmt.Errorf("sync/store: commit: %w", err)
	}
	return stored, nil
}

// MemCursorStore is a thread-safe in-memory CursorStore for tests. Devices
// must be pre-registered via SetDevice before AdvanceCursor can succeed.
type MemCursorStore struct {
	mu      sync.Mutex
	devices map[string]string // deviceID -> owner userID
	cursors map[string]Cursor // deviceID -> cursor
}

// NewMemCursorStore returns an empty in-memory store.
func NewMemCursorStore() *MemCursorStore {
	return &MemCursorStore{
		devices: make(map[string]string),
		cursors: make(map[string]Cursor),
	}
}

// SetDevice registers a device → owner mapping for tests.
func (m *MemCursorStore) SetDevice(deviceID, userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[deviceID] = userID
}

// Get returns the stored cursor (zero value if absent). Test helper.
func (m *MemCursorStore) Get(deviceID string) Cursor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cursors[deviceID]
}

// AdvanceCursor implements CursorStore in memory with the same monotonic
// semantics as the Postgres impl.
func (m *MemCursorStore) AdvanceCursor(_ context.Context, userID, deviceID string, requested Cursor) (Cursor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	owner, ok := m.devices[deviceID]
	if !ok {
		return Cursor{}, ErrDeviceNotFound
	}
	if owner != userID {
		return Cursor{}, ErrDeviceUserMismatch
	}
	existing := m.cursors[deviceID]
	if !existing.Less(requested) {
		return existing, nil
	}
	m.cursors[deviceID] = requested
	return requested, nil
}
