// Package store provides Postgres-backed persistence for nself-sync.
//
// Snapshot streaming: returns the user's current materialized state row-by-row
// via a pgx Rows cursor so memory stays bounded regardless of state size.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SnapshotRow is one row of materialized state streamed from np_sync_state.
type SnapshotRow struct {
	EventID       string          `json:"event_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Op            string          `json:"op"`
	HLCWallMs     int64           `json:"hlc_wall_ms"`
	HLCLamport    int64           `json:"hlc_lamport"`
	Payload       json.RawMessage `json:"payload"`
	SchemaVersion int             `json:"schema_version"`
}

// SnapshotCursor is the trailing metadata frame indicating the highest HLC
// included in the snapshot. Clients persist this and resume future pulls
// from it.
type SnapshotCursor struct {
	HLCWallMs  int64 `json:"hlc_wall_ms"`
	HLCLamport int64 `json:"hlc_lamport"`
}

// SnapshotStore is the minimal interface main.go uses to stream a snapshot.
// Implemented by *Store; allows tests to inject a fake.
type SnapshotStore interface {
	StreamSnapshot(ctx context.Context, userID string, cursorWall, cursorLam int64, fn func(SnapshotRow) error) (SnapshotCursor, error)
}

// Store wraps a pgx pool with sync-specific data methods.
type Store struct {
	pool *pgxpool.Pool
}

// New opens a pgx pool against the given DATABASE_URL.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("sync/store: parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("sync/store: connect: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Ping checks DB liveness.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// snapshotQuery returns the latest event per (entity_type, entity_id) for a
// user, optionally filtered to rows strictly greater than the provided HLC
// cursor for delta snapshots.
//
// Source: np_sync_state materialized view (defined in migration 001). The view
// is a deterministic projection of np_sync_events using DISTINCT ON.
const snapshotQuery = `
SELECT event_id::text, entity_type, entity_id::text, op,
       hlc_wall_ms, hlc_lamport, payload, schema_version
  FROM np_sync_state
 WHERE user_id = $1::uuid
   AND (hlc_wall_ms, hlc_lamport) > ($2::bigint, $3::bigint)
   AND op <> 'delete'
 ORDER BY hlc_wall_ms ASC, hlc_lamport ASC`

// StreamSnapshot iterates the user's current state, invoking fn once per row.
// Memory is bounded: pgx streams rows over a single connection cursor.
//
// cursorWall + cursorLam: when zero, returns full snapshot. Otherwise returns
// rows whose HLC strictly exceeds the cursor — used by reconnecting devices
// that have a partial state.
//
// Returns the highest HLC encountered, suitable for the trailing end frame.
// If no rows match, the returned cursor mirrors the input (cursorWall, cursorLam).
func (s *Store) StreamSnapshot(ctx context.Context, userID string, cursorWall, cursorLam int64, fn func(SnapshotRow) error) (SnapshotCursor, error) {
	rows, err := s.pool.Query(ctx, snapshotQuery, userID, cursorWall, cursorLam)
	if err != nil {
		return SnapshotCursor{}, fmt.Errorf("sync/store: query snapshot: %w", err)
	}
	defer rows.Close()

	maxWall, maxLam := cursorWall, cursorLam
	for rows.Next() {
		var r SnapshotRow
		if err := rows.Scan(&r.EventID, &r.EntityType, &r.EntityID, &r.Op,
			&r.HLCWallMs, &r.HLCLamport, &r.Payload, &r.SchemaVersion); err != nil {
			return SnapshotCursor{}, fmt.Errorf("sync/store: scan snapshot row: %w", err)
		}
		if err := fn(r); err != nil {
			return SnapshotCursor{}, err
		}
		if r.HLCWallMs > maxWall || (r.HLCWallMs == maxWall && r.HLCLamport > maxLam) {
			maxWall, maxLam = r.HLCWallMs, r.HLCLamport
		}
	}
	if err := rows.Err(); err != nil {
		return SnapshotCursor{}, fmt.Errorf("sync/store: iterate snapshot rows: %w", err)
	}

	return SnapshotCursor{HLCWallMs: maxWall, HLCLamport: maxLam}, nil
}

// Ensure *Store satisfies SnapshotStore at compile time.
var _ SnapshotStore = (*Store)(nil)

// rowsAdapter is here purely so pgx is referenced when building without
// running queries (tests use a fake store). Keeps the import graph clean.
var _ pgx.Rows = (pgx.Rows)(nil)
