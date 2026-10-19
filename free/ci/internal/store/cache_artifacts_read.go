package store

// Purpose: reads and retention of the artifact records (P7-CACHE-03, Epic D6):
// list, get, the missing flag, prune and the blob references a sweep keeps.
// Inputs: records.Filter, records.PruneSpec, ids.
// Outputs: records.Record values, the removed records of a prune.
// Constraints: a record changes after commit only in expires_at and missing
// (a SQL trigger enforces it); prune deletes rows in one transaction and
// never touches release-kind records without Release; staged rows of an
// attempt that is no longer running or finalizing are orphans and go with any
// prune; a dry run writes nothing.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
)

const artCols = `id,project,run_id,job,attempt_id,revision,decl,path,name,kind,sha256,size,platform,runner_id,isolation,producer,visibility,retention,release,epoch,state,missing,image,created_at,expires_at`

func authzVisibility(s string) authz.Visibility { return authz.Visibility(s) }

func scanArtifact(sc interface{ Scan(...any) error }) (r records.Record, err error) {
	var kind, vis, img string
	var created, exp int64
	var release, missing bool
	err = sc.Scan(&r.ID, &r.Project, &r.Run, &r.Job, &r.Attempt, &r.Revision, &r.Decl, &r.Path, &r.Name, &kind, &r.SHA256, &r.Size, &r.Platform, &r.RunnerID,
		&r.Isolation, &r.Producer, &vis, &r.Retention, &release, &r.Epoch, &r.State, &missing, &img, &created, &exp)
	if err != nil {
		return r, err
	}
	r.Kind, r.Visibility, r.Release, r.Missing, r.CreatedAt = records.Kind(kind), authzVisibility(vis), release, missing, time.Unix(0, created).UTC()
	if exp > 0 {
		t := time.Unix(0, exp).UTC()
		r.ExpiresAt = &t
	}
	if img != "" {
		r.Image = &records.ImageInfo{}
		if err = json.Unmarshal([]byte(img), r.Image); err != nil {
			return r, fmt.Errorf("artifact %s: image column: %w", r.ID, err)
		}
	}
	return r, nil
}

func (s *Store) artQuery(ctx context.Context, where string, args ...any) ([]records.Record, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT "+artCols+" FROM cache_artifact "+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []records.Record
	for rows.Next() {
		r, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ArtifactStaged returns the not yet committed records of an attempt.
func (s *Store) ArtifactStaged(ctx context.Context, attempt string) ([]records.Record, error) {
	return s.artQuery(ctx, "WHERE attempt_id=? AND state='staged' ORDER BY decl, name", attempt)
}

// ArtifactList returns records matching f, newest first.
func (s *Store) ArtifactList(ctx context.Context, f records.Filter) ([]records.Record, error) {
	var conds []string
	var args []any
	for _, c := range []struct{ col, val string }{{"project", f.Project}, {"run_id", f.Run}, {"attempt_id", f.Attempt}, {"name", f.Name}, {"kind", string(f.Kind)}} {
		if c.val != "" {
			conds, args = append(conds, c.col+"=?"), append(args, c.val)
		}
	}
	where := "WHERE state='committed'"
	if len(conds) > 0 {
		where += " AND " + strings.Join(conds, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	return s.artQuery(ctx, where+" ORDER BY created_at DESC, id LIMIT ?", append(args, limit)...)
}

// ArtifactGet returns one record by id.
func (s *Store) ArtifactGet(ctx context.Context, id string) (records.Record, bool, error) {
	recs, err := s.artQuery(ctx, "WHERE id=?", id)
	if err != nil || len(recs) == 0 {
		return records.Record{}, false, err
	}
	return recs[0], true, nil
}

// ArtifactSetMissing remembers whether a committed record's blob is gone.
func (s *Store) ArtifactSetMissing(ctx context.Context, id string, missing bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE cache_artifact SET missing=? WHERE id=? AND state='committed'", missing, id)
		return err
	})
}

// ArtifactUsage sums committed bytes plus the staged bytes of attempts that
// are still running or finalizing (their blobs are on disk already).
func (s *Store) ArtifactUsage(ctx context.Context, project string) (records.Usage, error) {
	var u records.Usage
	err := s.readers.QueryRowContext(ctx, `SELECT coalesce(sum(size),0),coalesce(sum(CASE WHEN release=1 THEN size END),0),
coalesce(sum(CASE WHEN project=? THEN size END),0),coalesce(sum(CASE WHEN project=? AND release=1 THEN size END),0) FROM cache_artifact
WHERE state='committed' OR attempt_id IN (SELECT id FROM attempt WHERE state IN ('running','finalizing'))`, project, project).
		Scan(&u.Total, &u.Release, &u.Project, &u.ProjectRelease)
	if err != nil {
		return u, err
	}
	var held, heldProject int64
	err = s.readers.QueryRowContext(ctx, `SELECT coalesce(sum(bytes),0),coalesce(sum(CASE WHEN project=? THEN bytes END),0) FROM cache_artifact_reserve
WHERE attempt_id IN (SELECT id FROM attempt WHERE state IN ('running','finalizing'))`, project).Scan(&held, &heldProject)
	u.Total, u.Project = u.Total+held, u.Project+heldProject
	return u, err
}

// ArtifactBlobRefs lists every blob a record (staged or committed) still needs.
func (s *Store) ArtifactBlobRefs(ctx context.Context) ([]records.BlobRef, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT DISTINCT project,sha256 FROM cache_artifact")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []records.BlobRef
	for rows.Next() {
		var b records.BlobRef
		if err = rows.Scan(&b.Project, &b.SHA256); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ArtifactPrune removes (or lists) the records spec allows.
func (s *Store) ArtifactPrune(ctx context.Context, spec records.PruneSpec) ([]records.Record, error) {
	now := spec.Now.UnixNano()
	cond := "(state='committed' AND release=0 AND expires_at>0 AND expires_at<=?)"
	args := []any{now}
	if spec.Release {
		cond += " OR (state='committed' AND release=1 AND ((expires_at>0 AND expires_at<=?) OR created_at<=?))"
		args = append(args, now, spec.Now.Add(-spec.OlderThan).UnixNano())
	}
	cond += " OR (state='staged' AND attempt_id NOT IN (SELECT id FROM attempt WHERE state IN ('running','finalizing')))"
	if spec.Project != "" { // a project-scoped prune never touches another project's rows
		cond = "(" + cond + ") AND project=?"
		args = append(args, spec.Project)
	}
	var removed []records.Record
	err := s.write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT "+artCols+" FROM cache_artifact WHERE "+cond+" ORDER BY created_at, id", args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			r, e := scanArtifact(rows)
			if e != nil {
				_ = rows.Close()
				return e
			}
			removed = append(removed, r)
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil || spec.DryRun || len(removed) == 0 {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM cache_artifact WHERE "+cond, args...); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, "artifacts", "artifact.prune", "artifacts", fmt.Sprintf("records=%d release=%t", len(removed), spec.Release))
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}
