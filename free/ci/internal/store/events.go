package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// RecordEvent stores one complete ci.events envelope. Sequence uniqueness makes
// accidental duplicate writers fail closed rather than corrupt a replay.
func (s *Store) RecordEvent(ctx context.Context, run string, seq int64, typ string, line []byte) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO event_index(run_id,seq,event_type,ref) VALUES(?,?,?,?)", run, seq, typ, string(line))
		return err
	})
}

// RunEvents returns the original envelopes in stream order, including evidence.
func (s *Store) RunEvents(ctx context.Context, run string) ([]json.RawMessage, error) {
	return s.RunEventsFrom(ctx, run, 0)
}

// RunEventsFrom reads only records at or after the requested sequence.
func (s *Store) RunEventsFrom(ctx context.Context, run string, from int64) ([]json.RawMessage, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT ref FROM event_index WHERE run_id=? AND (seq>=? OR event_type='result') ORDER BY seq", run, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
