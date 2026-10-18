package store

// Purpose: the recovery write of the run orchestrator (P7-CI-37): close the
// runs a dead coordinator left marked running.
// Inputs: the boot id of the caller. Outputs: the ids of the runs closed.
// Constraints: selection and closure are one transaction on the writer
// connection, so a coordinator that finishes or enqueues work meanwhile is
// never overwritten. A run is dead only when it has no live attempt, at least
// one attempt belongs to a stale coordinator (older boot or heartbeat past 3 x
// TTL) and none belongs to a live one; failure_class alone is not proof (a
// remote lease failure of a live coordinator also writes "lost").

import (
	"context"
	"database/sql"
)

// CloseDeadRuns marks lost every run that is still running although its
// coordinator is gone and nothing of it is live any more.
func (s *Store) CloseDeadRuns(ctx context.Context, currentBoot string) ([]string, error) {
	var closed []string
	err := s.write(ctx, func(tx *sql.Tx) error {
		cutoff := s.now().Add(-3 * s.ttl).UnixNano()
		rows, err := tx.QueryContext(ctx, `SELECT p.id FROM pipeline p WHERE p.status='running'
 AND NOT EXISTS (SELECT 1 FROM attempt a JOIN job j ON j.id=a.job_id WHERE j.pipeline_id=p.id AND a.state IN ('queued','leased','running','finalizing','lost'))
 AND EXISTS (SELECT 1 FROM attempt a JOIN job j ON j.id=a.job_id JOIN coordinator c ON c.id=a.coordinator_id WHERE j.pipeline_id=p.id AND (c.heartbeat_at<? OR c.boot_id!=?))
 AND NOT EXISTS (SELECT 1 FROM attempt a JOIN job j ON j.id=a.job_id JOIN coordinator c ON c.id=a.coordinator_id WHERE j.pipeline_id=p.id AND c.heartbeat_at>=? AND c.boot_id=?)
 ORDER BY p.id`, cutoff, currentBoot, cutoff, currentBoot)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			closed = append(closed, id)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		for _, id := range closed {
			if _, err = tx.ExecContext(ctx, "UPDATE pipeline SET status='lost',finished_at=? WHERE id=? AND status='running'", s.now().UnixNano(), id); err != nil {
				return err
			}
		}
		return nil
	})
	return closed, err
}
