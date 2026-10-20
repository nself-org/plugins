package store

// Purpose: the source trust class of an attempt's pipeline (D-1394): the fact a provider driver
//          dispatches on (which ref untrusted work may use).
// Inputs:  an attempt id.
// Outputs: the pipeline's source_trust; ok is false when the attempt (or its job or pipeline) is unknown.
// Constraints: read-only; an unknown attempt is ok=false, never a default, so the caller keeps its
//          safe (untrusted) default instead of reading a made-up class.

import (
	"context"
	"database/sql"
	"errors"
)

// AttemptSourceTrust returns the source trust class of the pipeline an attempt belongs to.
func (s *Store) AttemptSourceTrust(ctx context.Context, attemptID string) (trust string, ok bool, err error) {
	err = s.readers.QueryRowContext(ctx, `SELECT p.source_trust FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=?`, attemptID).Scan(&trust)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return trust, true, nil
}
