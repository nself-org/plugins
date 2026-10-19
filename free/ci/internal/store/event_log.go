package store

// Purpose: the stored ci.events log of a run (D-1061): the event_index table,
// one row per record, so `ci runs follow` can replay and tail a run that has no
// event file (a foreground run, or one on a coordinator).
// Inputs:  the run id, the record type and its encoded body (internal/events
// Record.Ref); this file does not look inside the body.
// Outputs: the seq the row got; the rows of a run from a seq on.
// Constraints: seq is per run, from 1, gap free, assigned inside the write
// transaction; rows go with their pipeline (prune.go); log records are never
// stored (they carry only byte counts).

import (
	"context"
	"database/sql"
)

// EventRow is one stored record: Type is the event type, Ref its encoded body.
type EventRow struct {
	Seq       int64
	Type, Ref string
}

// AppendEvent adds one record to the run's log and returns its seq.
func (s *Store) AppendEvent(ctx context.Context, runID, typ, ref string) (int64, error) {
	var seq int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(seq),0)+1 FROM event_index WHERE run_id=?`, runID).Scan(&seq); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO event_index(run_id,seq,event_type,ref) VALUES(?,?,?,?)`, runID, seq, typ, ref)
		return err
	})
	return seq, err
}

// EventRows returns the run's records with seq >= from, oldest first.
func (s *Store) EventRows(ctx context.Context, runID string, from int64) ([]EventRow, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT seq,event_type,ref FROM event_index WHERE run_id=? AND seq>=? ORDER BY seq`, runID, from)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []EventRow
	for rows.Next() {
		var r EventRow
		if err = rows.Scan(&r.Seq, &r.Type, &r.Ref); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
