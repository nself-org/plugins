package store

// Purpose: SQL for the artifact records (P7-CACHE-03, Epic D6): the stored
// facts of the producing attempt, the declarations, staged records and the
// capability gate every write goes through.
// Inputs: attempt id + lease epoch, records.Decl / records.Record values.
// Outputs: Facts, Decls, staged Records, *records.Error refusals.
// Constraints: migration set cache-artifacts references no other set's rows
// except the attempt table its guard trigger watches; every write re-reads
// attempt state and epoch inside the writer transaction; a stale capability
// commits one audit row and no record; the project, run, job and revision of
// a record come from the store, never from the caller; no env or secret value
// is stored.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

//go:embed migrations/cache-artifacts/*.sql
var cacheArtifactsMigrations embed.FS

var _ records.Ledger = (*Store)(nil)

func init() {
	if err := Register("cache-artifacts", cacheArtifactsMigrations); err != nil {
		panic(err)
	}
}

// artRefuse records the refusal's audit row inside tx and remembers the error.
func (s *Store) artRefuse(ctx context.Context, tx *sql.Tx, out *error, op, attempt, reason string) error {
	*out = records.Refuse(reason, "artifact "+op+" refused: the attempt no longer holds the write capability")
	return s.appendAudit(ctx, tx, "artifacts", "artifact."+op+".refused", attempt, "reason="+reason)
}

// artGate reports whether the attempt is in state want at the given epoch.
func (s *Store) artGate(ctx context.Context, tx *sql.Tx, out *error, op, attempt, want string, epoch int64) (bool, error) {
	var state string
	var cur int64
	err := tx.QueryRowContext(ctx, "SELECT state,epoch FROM attempt WHERE id=?", attempt).Scan(&state, &cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if state != "running" && state != "finalizing" {
		state = "ended"
	}
	switch {
	case state != want:
		return false, s.artRefuse(ctx, tx, out, op, attempt, records.ReasonAttempt)
	case cur != epoch:
		return false, s.artRefuse(ctx, tx, out, op, attempt, records.ReasonEpoch)
	}
	return true, nil
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func artFacts(ctx context.Context, tx rowQuerier, attempt string) (f records.Facts, err error) {
	var kind string
	err = tx.QueryRowContext(ctx, `SELECT p.project,p.id,j.name,p.revision,coalesce(r.id,coalesce(a.runner_id,'')),coalesce(r.platform,''),coalesce(r.isolation,''),j.kind
FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id LEFT JOIN runner_ref r ON r.id=a.runner_id WHERE a.id=?`, attempt).
		Scan(&f.Project, &f.Run, &f.Job, &f.Revision, &f.RunnerID, &f.Platform, &f.Isolation, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return f, coded("E604", "unknown attempt")
	}
	f.Release = kind == "release"
	return f, err
}

// ArtifactFacts returns the stored facts of an attempt.
func (s *Store) ArtifactFacts(ctx context.Context, attempt string) (records.Facts, error) {
	return artFacts(ctx, s.readers, attempt)
}

// ArtifactDeclare stores the declarations of an attempt (state running).
func (s *Store) ArtifactDeclare(ctx context.Context, attempt string, epoch int64, decls []records.Decl) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		if ok, err := s.artGate(ctx, tx, &refusal, "declare", attempt, "running", epoch); !ok || err != nil {
			return err
		}
		// a re-run of the same attempt row (retryable -> queued) supersedes the
		// uncommitted records of the earlier epoch; a second declare in the same
		// epoch, or an attempt that already committed records, is refused
		var have, current, committed int
		if err := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(epoch=?),0) FROM cache_artifact_decl WHERE attempt_id=?", epoch, attempt).Scan(&have, &current); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cache_artifact WHERE attempt_id=? AND state='committed'", attempt).Scan(&committed); err != nil {
			return err
		}
		if current > 0 || committed > 0 {
			return records.Failf(records.ReasonDeclaration, "attempt %s already declared or committed its artifacts", attempt)
		}
		if have > 0 {
			for _, q := range []string{"DELETE FROM cache_artifact WHERE attempt_id=? AND state='staged'", "DELETE FROM cache_artifact_decl WHERE attempt_id=?"} {
				if _, err := tx.ExecContext(ctx, q, attempt); err != nil {
					return err
				}
			}
		}
		for _, d := range decls {
			if _, err := tx.ExecContext(ctx, "INSERT INTO cache_artifact_decl(attempt_id,ord,path,name,kind,retention,visibility,image_ref,epoch,declared_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
				attempt, d.Ord, d.Path, d.Name, string(d.Kind), d.Retention, string(d.Visibility), d.ImageRef, epoch, s.now().UnixNano()); err != nil {
				return err
			}
		}
		return nil
	})
	return errors.Join(err, refusal)
}

// ArtifactDecls returns the declarations of an attempt in order.
func (s *Store) ArtifactDecls(ctx context.Context, attempt string) ([]records.Decl, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT ord,path,name,kind,retention,visibility,image_ref FROM cache_artifact_decl WHERE attempt_id=? ORDER BY ord", attempt)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []records.Decl
	for rows.Next() {
		var d records.Decl
		var kind, vis string
		if err = rows.Scan(&d.Ord, &d.Path, &d.Name, &kind, &d.Retention, &vis, &d.ImageRef); err != nil {
			return nil, err
		}
		d.Kind, d.Visibility = records.Kind(kind), authzVisibility(vis)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ArtifactStage stores the collected records of an attempt (state running).
func (s *Store) ArtifactStage(ctx context.Context, attempt string, epoch int64, recs []records.Record) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		if ok, err := s.artGate(ctx, tx, &refusal, "stage", attempt, "running", epoch); !ok || err != nil {
			return err
		}
		f, err := artFacts(ctx, tx, attempt)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM cache_artifact_reserve WHERE attempt_id=?", attempt); err != nil { // the staged rows now carry the claim
			return err
		}
		for _, r := range recs {
			var n int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM cache_artifact_decl WHERE attempt_id=? AND ord=?", attempt, r.Decl).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return records.Failf(records.ReasonDeclaration, "record %q names a declaration the attempt never made", r.Name)
			}
			img := ""
			if r.Image != nil {
				b, _ := json.Marshal(r.Image)
				img = string(b)
			}
			var exp int64
			if r.ExpiresAt != nil {
				exp = r.ExpiresAt.UnixNano()
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO cache_artifact(id,project,run_id,job,attempt_id,revision,decl,path,name,kind,sha256,size,platform,runner_id,isolation,producer,visibility,retention,release,epoch,state,image,created_at,expires_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'staged',?,?,?)`, r.ID, f.Project, f.Run, f.Job, attempt, f.Revision, r.Decl, r.Path, r.Name, string(r.Kind), r.SHA256, r.Size,
				r.Platform, orStr(r.RunnerID, f.RunnerID), r.Isolation, r.Producer, string(r.Visibility), r.Retention, f.Release, epoch, img, r.CreatedAt.UnixNano(), exp)
			if err != nil && strings.Contains(err.Error(), "UNIQUE") {
				return records.Failf(records.ReasonDuplicate, "attempt %s already holds an artifact named %q", attempt, r.Name)
			}
			if err != nil {
				return fmt.Errorf("stage artifact %q: %w", r.Name, err)
			}
		}
		return nil
	})
	return errors.Join(err, refusal)
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
