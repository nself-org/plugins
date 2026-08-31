// Package store — LISTEN/NOTIFY support for the WebSocket subscribe handler.
//
// Each WebSocket connection acquires a dedicated pgx conn for LISTEN; the
// existing pgxpool is used only for fetching event rows after a NOTIFY fires.
//
// Channel naming: np_sync_user_<user_id_no_dashes>. A row-insert trigger on
// np_sync_events fires NOTIFY <channel> so per-user streams stay isolated.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SyncEvent is one row of np_sync_events streamed to a WS subscriber.
type SyncEvent struct {
	EventID       string          `json:"event_id"`
	UserID        string          `json:"user_id"`
	DeviceID      string          `json:"device_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Op            string          `json:"op"`
	HLCWallMs     int64           `json:"hlc_wall_ms"`
	HLCLamport    int64           `json:"hlc_lamport"`
	Payload       json.RawMessage `json:"payload"`
	SchemaVersion int             `json:"schema_version"`
}

// ChannelForUser builds the per-user LISTEN channel name. UUID dashes are
// stripped so the result is a valid Postgres identifier prefix-safe form.
func ChannelForUser(userID string) string {
	safe := make([]byte, 0, len(userID))
	for i := 0; i < len(userID); i++ {
		c := userID[i]
		if c == '-' {
			continue
		}
		safe = append(safe, c)
	}
	return "np_sync_user_" + string(safe)
}

// Listener owns a dedicated pgx connection used exclusively for LISTEN/NOTIFY.
// Fetch-since queries reuse the Store's pool to avoid head-of-line blocking
// on the listening connection.
type Listener struct {
	conn  *pgx.Conn
	store *Store
}

// NewListener acquires a dedicated connection from the same DSN as the pool.
// The Store retains ownership of the pool; the Listener owns the dedicated
// conn and must be Closed by the caller.
func (s *Store) NewListener(ctx context.Context) (*Listener, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("sync/store: pool unavailable")
	}
	cfg := s.pool.Config().ConnConfig
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("sync/store: pgx connect: %w", err)
	}
	return &Listener{conn: conn, store: s}, nil
}

// Listen issues LISTEN on the dedicated connection.
func (l *Listener) Listen(ctx context.Context, channel string) error {
	if channel == "" {
		return errors.New("sync/store: empty listen channel")
	}
	_, err := l.conn.Exec(ctx, "LISTEN "+pgQuoteIdent(channel))
	return err
}

// WaitForNotification blocks until NOTIFY arrives or ctx is cancelled.
func (l *Listener) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	return l.conn.WaitForNotification(ctx)
}

// Close releases the dedicated pgx conn.
func (l *Listener) Close(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}
	return l.conn.Close(ctx)
}

// EventsSince returns events for userID strictly newer than cur, ordered
// (hlc_wall_ms ASC, hlc_lamport ASC). limit caps the result; <=0 → 500.
//
// Reads from the Store pool, not the listening connection.
func (l *Listener) EventsSince(ctx context.Context, userID string, cur Cursor, limit int) ([]SyncEvent, error) {
	if l == nil || l.store == nil {
		return nil, errors.New("sync/store: listener has no store")
	}
	return l.store.EventsSince(ctx, userID, cur, limit)
}

// EventsSince is the pool-backed query used by the listener and by tests that
// want to seed a stream without going through LISTEN/NOTIFY.
func (s *Store) EventsSince(ctx context.Context, userID string, cur Cursor, limit int) ([]SyncEvent, error) {
	if limit <= 0 {
		limit = 500
	}
	const q = `
		SELECT event_id::text, user_id::text, device_id::text,
		       entity_type, entity_id::text, op,
		       hlc_wall_ms, hlc_lamport, payload, schema_version
		  FROM np_sync_events
		 WHERE user_id = $1::uuid
		   AND (hlc_wall_ms, hlc_lamport) > ($2::bigint, $3::bigint)
		 ORDER BY hlc_wall_ms ASC, hlc_lamport ASC
		 LIMIT $4
	`
	rows, err := s.pool.Query(ctx, q, userID, cur.WallMs, cur.Lamport, limit)
	if err != nil {
		return nil, fmt.Errorf("sync/store: query events: %w", err)
	}
	defer rows.Close()

	out := make([]SyncEvent, 0, 16)
	for rows.Next() {
		var ev SyncEvent
		if err := rows.Scan(
			&ev.EventID, &ev.UserID, &ev.DeviceID,
			&ev.EntityType, &ev.EntityID, &ev.Op,
			&ev.HLCWallMs, &ev.HLCLamport,
			&ev.Payload, &ev.SchemaVersion,
		); err != nil {
			return nil, fmt.Errorf("sync/store: scan event: %w", err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync/store: rows: %w", err)
	}
	return out, nil
}

// pgQuoteIdent quotes a SQL identifier. The channel is built from a UUID with
// dashes removed, so this is defense-in-depth.
func pgQuoteIdent(s string) string {
	buf := make([]byte, 0, len(s)+2)
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			buf = append(buf, '"', '"')
			continue
		}
		buf = append(buf, s[i])
	}
	buf = append(buf, '"')
	return string(buf)
}
