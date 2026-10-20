package store

// Purpose: reads, claims, recovery and retention of the coordinator intake (P7-NODE-10, Epic D7); the
// enqueue transaction lives in node_intake.go.
// Inputs:  row ids, a claim owner, a stale window, a retention.
// Outputs: rows, counts, errors (E605 for a lost claim or an unknown row).
// Constraints: every claim change is owner-guarded; a row claimed IntakeMaxAttempts times is failed by
//          recovery instead of requeued, and a clean requeue does not spend an attempt.

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// IntakeGet reads one row.
func (s *Store) IntakeGet(ctx context.Context, id string) (IntakeRow, error) {
	r, err := scanIntake(s.readers.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, coded("E605", "unknown intake row")
	}
	return r, err
}

// DedupLookup finds the row a trigger with this semantic key would coalesce into: a live one, or (when
// completed is positive) one completed within that window. The enqueue repeats the check inside its own
// transaction; this read serves reports and tests.
func (s *Store) DedupLookup(ctx context.Context, key string, completed time.Duration) (IntakeRow, bool, error) {
	r, err := scanIntake(s.readers.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE semantic_key=? AND (state IN ('queued','dispatched') OR (?>0 AND state='done' AND finished_at>?)) ORDER BY seq DESC LIMIT 1`,
		key, int64(completed), s.now().UnixNano()-int64(completed)))
	if errors.Is(err, sql.ErrNoRows) {
		return IntakeRow{}, false, nil
	}
	return r, err == nil, err
}

// DedupPrune forgets finished rows, delivery ids and keys older than retention (0 means IntakeRetention)
// and returns how many rows it removed.
func (s *Store) DedupPrune(ctx context.Context, retention time.Duration) (int, error) {
	if retention <= 0 {
		retention = IntakeRetention
	}
	cut, n := s.now().UnixNano()-int64(retention), 0
	err := s.write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM intake WHERE state IN ('done','failed') AND finished_at<?`,
			`DELETE FROM intake_delivery WHERE created_at<?`,
			`DELETE FROM intake_idem WHERE created_at<?`,
		} {
			res, err := tx.ExecContext(ctx, q, cut)
			if err != nil {
				return err
			}
			c, _ := res.RowsAffected()
			n += int(c)
		}
		return nil
	})
	return n, err
}

// IntakeClaim hands the oldest queued row to owner, or reports false when the queue is empty.
func (s *Store) IntakeClaim(ctx context.Context, owner string) (IntakeRow, bool, error) {
	var r IntakeRow
	ok := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		row, err := scanIntake(tx.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE state='queued' ORDER BY seq LIMIT 1`))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		now := s.now().UnixNano()
		res, err := tx.ExecContext(ctx, `UPDATE intake SET state='dispatched',claimed_by=?,claim_at=?,attempts=attempts+1 WHERE id=? AND state='queued'`, owner, now, row.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return nil // another process took it: report an empty queue, the loop polls again
		}
		row.State, row.ClaimedBy, row.Attempts = IntakeDispatched, owner, row.Attempts+1
		r, ok = row, true
		return nil
	})
	return r, ok, err
}

// IntakeBeat refreshes the claim time of every row owner is dispatching.
func (s *Store) IntakeBeat(ctx context.Context, owner string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE intake SET claim_at=? WHERE state='dispatched' AND claimed_by=?`, s.now().UnixNano(), owner)
		return err
	})
}

// IntakeFinish ends a claimed row: done with the pipeline run id, or failed with the reason.
func (s *Store) IntakeFinish(ctx context.Context, id, owner, pipelineID, result string, ok bool) error {
	state := IntakeFailed
	if ok {
		state = IntakeDone
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE intake SET state=?,pipeline_id=?,result=?,finished_at=? WHERE id=? AND state='dispatched' AND claimed_by=?`,
			state, pipelineID, result, s.now().UnixNano(), id, owner)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return coded("E605", "intake row is no longer claimed by this coordinator")
		}
		return nil
	})
}

// IntakeRequeue returns a claimed row to the queue on shutdown; the claim does not count as an attempt.
func (s *Store) IntakeRequeue(ctx context.Context, id, owner string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE intake SET state='queued',claimed_by='',attempts=max(attempts-1,0) WHERE id=? AND state='dispatched' AND claimed_by=?`, id, owner)
		return err
	})
}

// IntakeRecover returns to the queue every row another owner claimed and has not refreshed within stale.
func (s *Store) IntakeRecover(ctx context.Context, owner string, stale time.Duration) (int, error) {
	n := 0
	err := s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		// a row claimed too often is failed, not queued again
		if _, err := tx.ExecContext(ctx, `UPDATE intake SET state='failed',finished_at=?,result='attempts exhausted' WHERE state='dispatched' AND claimed_by<>? AND claim_at<=? AND attempts>=?`,
			now, owner, now-int64(stale), IntakeMaxAttempts); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE intake SET state='queued',claimed_by='' WHERE state='dispatched' AND claimed_by<>? AND claim_at<=?`, owner, now-int64(stale))
		if err != nil {
			return err
		}
		c, _ := res.RowsAffected()
		n = int(c)
		return nil
	})
	return n, err
}

// IntakeStats counts queued and dispatched rows and the leases that have not expired.
func (s *Store) IntakeStats(ctx context.Context) (IntakeStats, error) {
	var st IntakeStats
	err := s.readers.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM intake WHERE state='queued'),
 (SELECT count(*) FROM intake WHERE state='dispatched'),
 (SELECT count(*) FROM lease WHERE expires_at>?)`, s.now().UnixNano()).Scan(&st.Queued, &st.Dispatched, &st.Leases)
	return st, err
}
