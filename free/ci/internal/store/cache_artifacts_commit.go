package store

// Purpose: the commit of staged artifact records (P7-CACHE-03, Epic D6): the
// only transition from staged to committed, taken in the finalizing state at
// the current epoch, after the caller re-hashed every blob.
// Inputs: attempt id, epoch, the verified staged records, a records.Policy.
// Outputs: the 80 % warning text; *records.Error (E711 stale capability,
// E712 size_cap) after one audit row.
// Constraints: one writer transaction re-reads the attempt, the staged rows
// and the committed byte sums; the rows the caller verified must be exactly
// the rows that are staged now; released-kind bytes count toward the cap.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

// artUsage sums committed bytes: the store, release-kind, the project, and the
// project's release-kind.
func artUsage(ctx context.Context, tx *sql.Tx, project string) (u records.Usage, err error) {
	err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(size),0),coalesce(sum(CASE WHEN release=1 THEN size END),0),
coalesce(sum(CASE WHEN project=? THEN size END),0),coalesce(sum(CASE WHEN project=? AND release=1 THEN size END),0) FROM cache_artifact WHERE state='committed'`, project, project).
		Scan(&u.Total, &u.Release, &u.Project, &u.ProjectRelease)
	return u, err
}

// ArtifactCommit commits the verified staged records of an attempt.
func (s *Store) ArtifactCommit(ctx context.Context, attempt string, epoch int64, verified []records.Record, pol records.Policy) (string, error) {
	var refusal error
	var warning string
	err := s.write(ctx, func(tx *sql.Tx) error {
		if ok, err := s.artGate(ctx, tx, &refusal, "commit", attempt, "finalizing", epoch); !ok || err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT id,sha256,size,project FROM cache_artifact WHERE attempt_id=? AND state='staged' ORDER BY id", attempt)
		if err != nil {
			return err
		}
		staged := map[string]records.Record{}
		var project string
		for rows.Next() {
			var r records.Record
			if err = rows.Scan(&r.ID, &r.SHA256, &r.Size, &project); err != nil {
				_ = rows.Close()
				return err
			}
			staged[r.ID] = r
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return err
		}
		var add int64
		for _, v := range verified {
			r, ok := staged[v.ID]
			if !ok || r.SHA256 != v.SHA256 || r.Size != v.Size {
				return records.Failf(records.ReasonDigest, "record %q is not the staged record that was verified", v.Name)
			}
			add += r.Size
			delete(staged, v.ID)
		}
		if len(staged) > 0 {
			return records.Failf(records.ReasonUncommitted, "%d staged record(s) of %s were not verified", len(staged), attempt)
		}
		u, err := artUsage(ctx, tx, project)
		if err != nil {
			return err
		}
		if warning, err = pol.CheckCaps(u, add); err != nil {
			refusal = err
			return s.appendAudit(ctx, tx, "artifacts", "artifact.commit.refused", attempt, "reason="+records.ReasonSizeCap)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE cache_artifact SET state='committed' WHERE attempt_id=? AND state='staged'", attempt); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, "artifacts", "artifact.commit", attempt, fmt.Sprintf("records=%d bytes=%d", len(verified), add))
	})
	return warning, errors.Join(err, refusal)
}
