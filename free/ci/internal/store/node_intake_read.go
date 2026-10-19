package store

// Purpose: the read-only intake queries the coordinator user API needs (P7-NODE-19): the newest rows for
// `ci runs`, the row that stands for a pipeline and the dispatched rows of one revision, which is how a
// run is tied to the owner/repo project its caller may see (a pipeline row keeps only the checkout path).
// Inputs:  a limit, or a pipeline id.
// Outputs: IntakeRow values (newest first for the list).
// Constraints: read-only; no new table or migration.

import (
	"context"
	"database/sql"
	"errors"
)

// IntakeList returns the newest limit rows (at most 500), newest first.
func (s *Store) IntakeList(ctx context.Context, limit int) ([]IntakeRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.readers.QueryContext(ctx, `SELECT `+intakeCols+` FROM intake ORDER BY seq DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []IntakeRow
	for rows.Next() {
		r, err := scanIntake(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// IntakeByPipeline finds the row whose finished dispatch produced the pipeline.
func (s *Store) IntakeByPipeline(ctx context.Context, pipelineID string) (IntakeRow, bool, error) {
	r, err := scanIntake(s.readers.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE pipeline_id=? ORDER BY seq DESC LIMIT 1`, pipelineID))
	if errors.Is(err, sql.ErrNoRows) {
		return IntakeRow{}, false, nil
	}
	return r, err == nil, err
}

// IntakeDispatchedByRevision returns every dispatched row of revision that names no pipeline yet (no
// limit: ownership of an in-flight run is decided over all of them, never over a window).
func (s *Store) IntakeDispatchedByRevision(ctx context.Context, revision string) ([]IntakeRow, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE state='dispatched' AND pipeline_id='' AND revision=? ORDER BY seq`, revision)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []IntakeRow
	for rows.Next() {
		r, err := scanIntake(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
