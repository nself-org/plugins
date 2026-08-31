// Package pty implements PTY session management for plugin-pty.
//
// Purpose: Create, track, and close PTY sessions in Postgres.
//          Enforce per-tenant session limits.
// Inputs:  Postgres pool, source_account_id from JWT header.
// Outputs: Session rows; audit log entries.
// Constraints:
//   - All DB operations scoped by source_account_id (Multi-App Isolation).
//   - Max PTY sessions per tenant enforced before spawn.
//   - ctx.License.Valid() checked in handler layer before DB access.
//
// SPORT: F04-PLUGIN-INVENTORY-PRO.md — plugin-pty row
package pty

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Session represents a row in np_pty_sessions.
type Session struct {
	ID              string
	SourceAccountID string
	ClientID        string
	Status          string
	Cols            int
	Rows            int
	StartedAt       time.Time
	EndedAt         *time.Time
}

// DB wraps the Postgres connection pool for PTY operations.
type DB struct {
	Pool *pgxpool.Pool
}

// ActiveCount returns the number of active PTY sessions for the given account.
func (d *DB) ActiveCount(ctx context.Context, sourceAccountID string) (int, error) {
	var count int
	err := d.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_pty_sessions WHERE source_account_id = $1 AND status = 'active'`,
		sourceAccountID,
	).Scan(&count)
	return count, err
}

// CreateSession inserts a new PTY session row and returns its ID.
func (d *DB) CreateSession(ctx context.Context, sourceAccountID, clientID string, cols, rows int) (string, error) {
	var id string
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO np_pty_sessions (source_account_id, client_id, cols, rows)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		sourceAccountID, clientID, cols, rows,
	).Scan(&id)
	return id, err
}

// GetSession fetches a session by ID, enforcing source_account_id ownership.
func (d *DB) GetSession(ctx context.Context, id, sourceAccountID string) (*Session, error) {
	row := d.Pool.QueryRow(ctx,
		`SELECT id, source_account_id, client_id, status, cols, rows, started_at, ended_at
		 FROM np_pty_sessions WHERE id = $1 AND source_account_id = $2`,
		id, sourceAccountID,
	)
	var s Session
	if err := row.Scan(&s.ID, &s.SourceAccountID, &s.ClientID, &s.Status, &s.Cols, &s.Rows, &s.StartedAt, &s.EndedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// CloseSession marks a session as closed.
func (d *DB) CloseSession(ctx context.Context, id, sourceAccountID string) error {
	ct, err := d.Pool.Exec(ctx,
		`UPDATE np_pty_sessions SET status = 'closed', ended_at = NOW()
		 WHERE id = $1 AND source_account_id = $2 AND status = 'active'`,
		id, sourceAccountID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("session not found or already closed")
	}
	return nil
}

// Audit appends an event to np_pty_audit_log.
func (d *DB) Audit(ctx context.Context, sourceAccountID, sessionID, eventType, detail string) {
	_, _ = d.Pool.Exec(ctx,
		`INSERT INTO np_pty_audit_log (source_account_id, session_id, event_type, detail)
		 VALUES ($1, $2::uuid, $3, $4)`,
		sourceAccountID, sessionID, eventType, detail,
	)
}
