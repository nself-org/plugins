package store

// Purpose: the recovery read of the run orchestrator (P7-CI-37): which runs a
// dead coordinator left marked running.
// Inputs: none. Outputs: run ids.
// Constraints: a live run never has an attempt whose failure class is "lost"
// (only coordinator recovery writes it), so a running run with such an attempt
// and nothing live left belongs to a dead coordinator.

import (
	"context"
	"database/sql"
)

// DeadRuns lists the runs still marked running whose attempts were lost to a
// dead coordinator and none of whose attempts is live any more.
func (s *Store) DeadRuns(ctx context.Context) ([]string, error) {
	return collect(ctx, s, func(r *sql.Rows) (id string, err error) {
		err = r.Scan(&id)
		return
	}, `SELECT p.id FROM pipeline p WHERE p.status='running'
 AND EXISTS (SELECT 1 FROM attempt a JOIN job j ON j.id=a.job_id WHERE j.pipeline_id=p.id AND a.failure_class='lost')
 AND NOT EXISTS (SELECT 1 FROM attempt a JOIN job j ON j.id=a.job_id WHERE j.pipeline_id=p.id AND a.state IN ('queued','leased','running','finalizing','lost'))
 ORDER BY p.id`)
}
