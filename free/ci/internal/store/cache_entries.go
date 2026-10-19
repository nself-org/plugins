package store

// Purpose: SQL for the cache entries (P7-CACHE-01, Epic D3/D4), write side: the
// job facts and planned save set recorded at restore time, the recorded result,
// and the commit transaction that is the only write capability. Reads, pruning
// and usage rows are in cache_entries_read.go.
// Inputs: attempt id + capability epoch, entries.Facts, entries.CommitRequest.
// Outputs: *entries.Refused (E711) or authz's *Denied (E684); one audit row per
// refusal, committed with no entry.
// Constraints: migration set cache-entries has no foreign key into another set.
// The capability epoch is the fence of the attempt's CURRENT lease row
// (LeaseGrant) when it has one, else the attempt's transition counter
// (attempt.epoch); every re-queue moves it. A plan records the transition counter
// it was made at and a result is accepted only one transition later (running to
// finalizing), so a plan or result from an earlier run never authorises a later one. CacheCommit re-reads attempt state, epoch,
// result, stored facts and the planned set in one writer transaction and
// recomputes the namespace through authz.CacheNamespace (the caller's value is
// only compared). No env or secret value is ever stored.

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

//go:embed migrations/cache-entries/*.sql
var cacheEntriesMigrations embed.FS

func init() {
	if err := Register("cache-entries", cacheEntriesMigrations); err != nil {
		panic(err)
	}
}

// cacheRefuse records the refusal's audit row inside tx and remembers the error.
func (s *Store) cacheRefuse(ctx context.Context, tx *sql.Tx, out *error, op, attempt, reason string) error {
	*out = &entries.Refused{Reason: reason}
	return s.appendAudit(ctx, tx, "cache", "cache."+op+".refused", attempt, "reason="+reason)
}

// cacheGate re-reads the attempt and reports whether it is in state want at the
// capability epoch (any state but running/finalizing reads "ended"); otherwise
// it records the refusal (attempt_state or epoch). cur is the capability epoch,
// cas the attempt's transition counter.
func (s *Store) cacheGate(ctx context.Context, tx *sql.Tx, out *error, op, attempt, want string, epoch int64) (cur, cas int64, ok bool, err error) {
	var state string
	err = tx.QueryRowContext(ctx, "SELECT a.state,a.epoch,coalesce((SELECT l.epoch FROM lease l WHERE l.attempt_id=a.id),0) FROM attempt a WHERE a.id=?", attempt).Scan(&state, &cas, &cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, err
	}
	if cur == 0 {
		cur = cas
	}
	if state != "running" && state != "finalizing" {
		state = "ended"
	}
	switch {
	case state != want:
		return cur, cas, false, s.cacheRefuse(ctx, tx, out, op, attempt, entries.ReasonAttemptState)
	case cur != epoch:
		return cur, cas, false, s.cacheRefuse(ctx, tx, out, op, attempt, entries.ReasonEpoch)
	}
	return cur, cas, true, nil
}

// cacheRow is a cache_attempt row.
type cacheRow struct {
	f                    entries.Facts
	resultEpoch, planCAS int64
	passed, found        bool
}

func cacheFacts(ctx context.Context, tx *sql.Tx, id string) (r cacheRow, err error) {
	f := &r.f
	err = tx.QueryRowContext(ctx, "SELECT project,trust,protection,head_repo,pr,secrets_declared,private_deps,passed,result_epoch,plan_cas FROM cache_attempt WHERE attempt_id=?", id).
		Scan(&f.Project, &f.Trust, &f.Protection, &f.HeadRepo, &f.PR, &f.SecretsDeclared, &f.PrivateDeps, &r.passed, &r.resultEpoch, &r.planCAS)
	if errors.Is(err, sql.ErrNoRows) {
		return cacheRow{resultEpoch: -1, planCAS: -1}, nil
	}
	r.found = err == nil
	return r, err
}

func execAll(ctx context.Context, tx *sql.Tx, stmts ...[]any) error {
	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st[0].(string), st[1:]...); err != nil {
			return err
		}
	}
	return nil
}

// CacheRecordPlan records, while the attempt runs at epoch, the job facts and
// the planned save keys (replacing any earlier set and clearing any earlier
// result, so a re-run starts clean). The facts must match the pipeline row the
// attempt belongs to (project and source trust) and an earlier record.
func (s *Store) CacheRecordPlan(ctx context.Context, attempt string, epoch int64, f entries.Facts, keys []string) error {
	if _, err := entries.NamespaceOf(f); err != nil {
		return err
	}
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		_, cas, ok, err := s.cacheGate(ctx, tx, &refusal, "plan", attempt, "running", epoch)
		if !ok || err != nil {
			return err
		}
		var project, trust string
		if err := tx.QueryRowContext(ctx, "SELECT p.project,p.source_trust FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=?", attempt).Scan(&project, &trust); err != nil {
			return err
		}
		old, err := cacheFacts(ctx, tx, attempt)
		if err != nil {
			return err
		}
		if project != f.Project || trust != f.Trust || old.found && old.f != f {
			return s.cacheRefuse(ctx, tx, &refusal, "plan", attempt, entries.ReasonNamespace)
		}
		stmts := [][]any{{"INSERT OR IGNORE INTO cache_attempt(attempt_id,project,trust,protection,head_repo,pr,secrets_declared,private_deps,recorded_at) VALUES (?,?,?,?,?,?,?,?,?)",
			attempt, f.Project, f.Trust, f.Protection, f.HeadRepo, f.PR, f.SecretsDeclared, f.PrivateDeps, s.now().UnixNano()},
			{"UPDATE cache_attempt SET passed=0,result_epoch=-1,plan_cas=? WHERE attempt_id=?", cas, attempt}, {"DELETE FROM cache_planned WHERE attempt_id=?", attempt}}
		for _, k := range keys {
			stmts = append(stmts, []any{"INSERT OR IGNORE INTO cache_planned(attempt_id,cache_key) VALUES (?,?)", attempt, k})
		}
		return execAll(ctx, tx, stmts...)
	})
	return errors.Join(err, refusal)
}

// CacheRecordResult records the attempt's result while it is finalizing at
// epoch; the pass is bound to that epoch and a failure drops the planned set.
func (s *Store) CacheRecordResult(ctx context.Context, attempt string, epoch int64, passed bool) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		_, cas, ok, err := s.cacheGate(ctx, tx, &refusal, "result", attempt, "finalizing", epoch)
		if !ok || err != nil {
			return err
		}
		row, err := cacheFacts(ctx, tx, attempt)
		switch {
		case err != nil:
			return err
		case !row.found || row.planCAS+1 != cas: // no plan recorded in this run
			return s.cacheRefuse(ctx, tx, &refusal, "result", attempt, entries.ReasonNotPlanned)
		case row.resultEpoch == epoch && row.passed != passed: // a result is write-once per run
			return s.cacheRefuse(ctx, tx, &refusal, "result", attempt, entries.ReasonAttemptState)
		}
		if err = execAll(ctx, tx, []any{"UPDATE cache_attempt SET passed=?,result_epoch=? WHERE attempt_id=?", passed, epoch, attempt}); err != nil {
			return err
		}
		if passed {
			return nil
		}
		return execAll(ctx, tx, []any{"DELETE FROM cache_planned WHERE attempt_id=?", attempt})
	})
	return errors.Join(err, refusal)
}

// CacheCommit commits the entry or returns the refusal; nothing is visible
// before. Refusal order: attempt state, epoch and a pass recorded at this epoch
// (E711), authz.CheckWrite on the claimed namespace (E684), then the manifest's
// namespace, the planned key and the size cap, then Check (E711 as returned).
func (s *Store) CacheCommit(ctx context.Context, c entries.CommitRequest) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		refuse := func(reason string) error { return s.cacheRefuse(ctx, tx, &refusal, "commit", c.Attempt, reason) }
		cur, _, ok, err := s.cacheGate(ctx, tx, &refusal, "commit", c.Attempt, "finalizing", c.Epoch)
		if !ok || err != nil {
			return err
		}
		row, err := cacheFacts(ctx, tx, c.Attempt)
		if err != nil {
			return err
		}
		f, passed := row.f, row.passed
		if !row.found || !passed || row.resultEpoch != cur {
			return refuse(entries.ReasonAttemptState)
		}
		ns, err := entries.NamespaceOf(f)
		if err != nil {
			return refuse(entries.ReasonNamespace)
		}
		job := authz.Job{Namespace: ns, State: authz.StateFinalizing, Passed: passed, Capability: authz.Capability{Attempt: c.Attempt, Epoch: c.Epoch}}
		if denied := authz.CheckWrite(job, c.Claimed, authz.Capability{Attempt: c.Attempt, Epoch: cur}); denied != nil {
			var d *authz.Denied
			errors.As(denied, &d)
			refusal = denied
			return s.appendAudit(ctx, tx, "cache", "cache.commit.denied", c.Attempt, "reason="+string(d.Reason))
		}
		var planned int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM cache_planned WHERE attempt_id=? AND cache_key=?", c.Attempt, c.Manifest.Key).Scan(&planned); err != nil {
			return err
		}
		e, total := c.Manifest, int64(0)
		for _, b := range e.Blobs {
			total += b.Size
		}
		var foreign int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM cache_entry WHERE cache_key=? AND namespace<>?", e.Key, e.Namespace).Scan(&foreign); err != nil {
			return err
		}
		switch {
		case !bytes.Equal(e.Namespace, ns.Canonical()) || foreign != 0: // never replace another namespace's entry
			return refuse(entries.ReasonNamespace)
		case planned != 1:
			return refuse(entries.ReasonNotPlanned)
		case c.MaxEntryBytes > 0 && total > c.MaxEntryBytes:
			return refuse(entries.ReasonSizeCap)
		}
		if c.Check != nil {
			var r *entries.Refused
			if err = c.Check(); errors.As(err, &r) {
				return refuse(r.Reason)
			} else if err != nil {
				return err
			}
		}
		now := s.now().UnixNano()
		stmts := [][]any{{"DELETE FROM cache_entry WHERE cache_key=? AND namespace=?", e.Key, e.Namespace},
			{"INSERT INTO cache_entry(cache_key,namespace,spec,attempt_id,epoch,source_trust,secrets_received,private_deps,scanned_clean,bytes,created_at,last_used_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
				e.Key, e.Namespace, e.Spec, c.Attempt, cur, f.Trust, f.SecretsDeclared, f.PrivateDeps, e.Writer.ScannedClean, total, now, now},
			{"DELETE FROM cache_planned WHERE attempt_id=? AND cache_key=?", c.Attempt, e.Key}}
		for _, b := range e.Blobs {
			stmts = append(stmts, []any{"INSERT INTO cache_entry_blob(cache_key,path,sha256,size) VALUES (?,?,?,?)", e.Key, b.Path, b.SHA256, b.Size})
		}
		return execAll(ctx, tx, stmts...)
	})
	return errors.Join(err, refusal)
}
