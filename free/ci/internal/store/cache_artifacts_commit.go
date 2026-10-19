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

// ArtifactReserve claims bytes of the store for a running attempt before any is
// written: it re-reads the live usage (committed, staged and other attempts'
// claims) inside the writer transaction, applies the caps, and keeps the claim
// until the attempt stages its records or ends. A later reserve of the same
// attempt replaces the earlier one.
func (s *Store) ArtifactReserve(ctx context.Context, attempt string, epoch int64, bytes int64, pol records.Policy) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		if ok, err := s.artGate(ctx, tx, &refusal, "reserve", attempt, "running", epoch); !ok || err != nil {
			return err
		}
		f, err := artFacts(ctx, tx, attempt)
		if err != nil {
			return err
		}
		u, err := artUsage(ctx, tx, f.Project)
		if err != nil {
			return err
		}
		var held, heldProject int64
		if err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(bytes),0),coalesce(sum(CASE WHEN project=? THEN bytes END),0) FROM cache_artifact_reserve
WHERE attempt_id<>? AND attempt_id IN (SELECT id FROM attempt WHERE state IN ('running','finalizing'))`, f.Project, attempt).Scan(&held, &heldProject); err != nil {
			return err
		}
		var staged, stagedProject int64
		if err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(size),0),coalesce(sum(CASE WHEN project=? THEN size END),0) FROM cache_artifact
WHERE state='staged' AND attempt_id<>? AND attempt_id IN (SELECT id FROM attempt WHERE state IN ('running','finalizing'))`, f.Project, attempt).Scan(&staged, &stagedProject); err != nil {
			return err
		}
		u.Total, u.Project = u.Total+held+staged, u.Project+heldProject+stagedProject
		if _, err = pol.CheckCaps(u, bytes); err != nil {
			refusal = err
			return s.appendAudit(ctx, tx, "artifacts", "artifact.reserve.refused", attempt, "reason="+records.ReasonSizeCap)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO cache_artifact_reserve(attempt_id,project,bytes,epoch,reserved_at) VALUES (?,?,?,?,?) ON CONFLICT(attempt_id) DO UPDATE SET bytes=excluded.bytes, epoch=excluded.epoch, reserved_at=excluded.reserved_at",
			attempt, f.Project, bytes, epoch, s.now().UnixNano())
		return err
	})
	return errors.Join(err, refusal)
}
