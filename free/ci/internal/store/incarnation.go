package store

// Purpose: the coordinator incarnation fenced remote leases carry (P7-NODE-20, Epic D6): the highest one in
//          the coordinator table. A restore bumps every row, so a lease granted after a restore carries an
//          incarnation above every lease granted before the backup, whichever process id serves.
// Inputs:  none. Outputs: the highest incarnation, at least 1. Constraints: read-only.

import (
	"context"
	"database/sql"
)

// HighestIncarnation returns the highest incarnation of any registered coordinator, and 1 when there is none.
func (s *Store) HighestIncarnation(ctx context.Context) (int64, error) {
	var n int64
	if err := s.readers.QueryRowContext(ctx, `SELECT coalesce(max(incarnation),0) FROM coordinator`).Scan(&n); err != nil {
		return 1, err
	}
	return max(n, 1), nil
}

// RaiseIncarnation makes the highest incarnation at least floor and returns it. It adds a placeholder row
// (no process, never live) when no existing row can carry the value. A restore uses it so the restored
// state never fences below the coordinator it replaced.
func (s *Store) RaiseIncarnation(ctx context.Context, floor int64) (int64, error) {
	var out int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(incarnation),0) FROM coordinator`).Scan(&out); err != nil {
			return err
		}
		if out >= floor {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO coordinator(id,pid,process_start,boot_id,incarnation,heartbeat_at) VALUES ('restore-floor',0,'','',?,0) ON CONFLICT(id) DO UPDATE SET incarnation=excluded.incarnation`, floor)
		out = floor
		return err
	})
	return out, err
}
