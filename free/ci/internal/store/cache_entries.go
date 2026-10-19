package store

// Purpose: SQL for the cache entries (P7-CACHE-01, Epic D3/D4): the facts and
// planned save set recorded at restore time, the commit transaction that is the
// only write capability, entry reads, LRU pruning and usage rows.
// Inputs: attempt id + lease epoch, entries.Facts, Manifest, Use.
// Outputs: Manifest values, *entries.Refused (E711) or authz's *Denied (E684).
// Constraints: migration set cache-entries has no foreign key into another set;
// CacheCommit re-reads attempt state, epoch, result, stored job facts and the
// planned save set inside one writer transaction and recomputes the namespace
// through authz.CacheNamespace (the caller's value is only compared); a refusal
// commits one audit row and no entry; no env or secret value is ever stored.

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
// given epoch (any state but running/finalizing reads "ended"); otherwise it
// records the refusal (attempt_state or epoch).
func (s *Store) cacheGate(ctx context.Context, tx *sql.Tx, out *error, op, attempt, want string, epoch int64) (bool, error) {
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
		return false, s.cacheRefuse(ctx, tx, out, op, attempt, "attempt_state")
	case cur != epoch:
		return false, s.cacheRefuse(ctx, tx, out, op, attempt, "epoch")
	}
	return true, nil
}

func cacheFacts(ctx context.Context, tx *sql.Tx, id string) (f entries.Facts, passed, ok bool, err error) {
	err = tx.QueryRowContext(ctx, "SELECT project,trust,protection,head_repo,pr,secrets_declared,private_deps,passed FROM cache_attempt WHERE attempt_id=?", id).
		Scan(&f.Project, &f.Trust, &f.Protection, &f.HeadRepo, &f.PR, &f.SecretsDeclared, &f.PrivateDeps, &passed)
	if errors.Is(err, sql.ErrNoRows) {
		return f, false, false, nil
	}
	return f, passed, err == nil, err
}

func execAll(ctx context.Context, tx *sql.Tx, stmts ...[]any) error {
	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st[0].(string), st[1:]...); err != nil {
			return err
		}
	}
	return nil
}

// CacheRecordPlan stores the job facts and planned save keys at restore time
// (state running, current epoch). Facts are first-write-wins: a different
// second record is refused.
func (s *Store) CacheRecordPlan(ctx context.Context, attempt string, epoch int64, f entries.Facts, keys []string) error {
	if _, err := entries.NamespaceOf(f); err != nil {
		return err
	}
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		if ok, err := s.cacheGate(ctx, tx, &refusal, "plan", attempt, "running", epoch); !ok || err != nil {
			return err
		}
		old, _, found, err := cacheFacts(ctx, tx, attempt)
		if err != nil {
			return err
		}
		if found && old != f {
			return s.cacheRefuse(ctx, tx, &refusal, "plan", attempt, "namespace")
		}
		stmts := [][]any{{"INSERT OR IGNORE INTO cache_attempt(attempt_id,project,trust,protection,head_repo,pr,secrets_declared,private_deps,recorded_at) VALUES (?,?,?,?,?,?,?,?,?)",
			attempt, f.Project, f.Trust, f.Protection, f.HeadRepo, f.PR, f.SecretsDeclared, f.PrivateDeps, s.now().UnixNano()}}
		for _, k := range keys {
			stmts = append(stmts, []any{"INSERT OR IGNORE INTO cache_planned(attempt_id,cache_key) VALUES (?,?)", attempt, k})
		}
		return execAll(ctx, tx, stmts...)
	})
	return errors.Join(err, refusal)
}

// CacheRecordResult records the attempt's result (state finalizing, current epoch).
func (s *Store) CacheRecordResult(ctx context.Context, attempt string, epoch int64, passed bool) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		if ok, err := s.cacheGate(ctx, tx, &refusal, "result", attempt, "finalizing", epoch); !ok || err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "UPDATE cache_attempt SET passed=? WHERE attempt_id=?", passed, attempt)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return s.cacheRefuse(ctx, tx, &refusal, "result", attempt, "not_planned")
		}
		return nil
	})
	return errors.Join(err, refusal)
}

// CacheCommit commits the entry or returns the refusal; nothing is visible before.
func (s *Store) CacheCommit(ctx context.Context, c entries.CommitRequest) error {
	var refusal error
	err := s.write(ctx, func(tx *sql.Tx) error {
		refuse := func(reason string) error { return s.cacheRefuse(ctx, tx, &refusal, "commit", c.Attempt, reason) }
		f, passed, found, err := cacheFacts(ctx, tx, c.Attempt)
		if err != nil {
			return err
		}
		if !found || !passed {
			if ok, err := s.cacheGate(ctx, tx, &refusal, "commit", c.Attempt, "finalizing", c.Epoch); !ok || err != nil {
				return err
			}
			return refuse("attempt_state")
		}
		if ok, err := s.cacheGate(ctx, tx, &refusal, "commit", c.Attempt, "finalizing", c.Epoch); !ok || err != nil {
			return err
		}
		ns, err := entries.NamespaceOf(f)
		if err != nil {
			return refuse("namespace")
		}
		job := authz.Job{Namespace: ns, State: authz.StateFinalizing, Passed: passed, Capability: authz.Capability{Attempt: c.Attempt, Epoch: c.Epoch}}
		if denied := authz.CheckWrite(job, c.Claimed, authz.Capability{Attempt: c.Attempt, Epoch: c.Epoch}); denied != nil {
			var d *authz.Denied
			errors.As(denied, &d)
			refusal = denied
			return s.appendAudit(ctx, tx, "cache", "cache.commit.denied", c.Attempt, "reason="+string(d.Reason))
		}
		var planned int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM cache_planned WHERE attempt_id=? AND cache_key=?", c.Attempt, c.Manifest.Key).Scan(&planned); err != nil {
			return err
		}
		var total int64
		for _, b := range c.Manifest.Blobs {
			total += b.Size
		}
		switch {
		case !bytes.Equal(c.Manifest.Namespace, ns.Canonical()):
			return refuse("namespace")
		case planned != 1:
			return refuse("not_planned")
		case c.MaxEntryBytes > 0 && total > c.MaxEntryBytes:
			return refuse("size_cap")
		}
		e, now := c.Manifest, s.now().UnixNano()
		stmts := [][]any{{"DELETE FROM cache_entry WHERE cache_key=?", e.Key},
			{"INSERT INTO cache_entry(cache_key,namespace,spec,attempt_id,epoch,source_trust,secrets_received,private_deps,scanned_clean,bytes,created_at,last_used_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
				e.Key, e.Namespace, e.Spec, c.Attempt, c.Epoch, f.Trust, f.SecretsDeclared, f.PrivateDeps, e.Writer.ScannedClean, total, now, now},
			{"DELETE FROM cache_planned WHERE attempt_id=? AND cache_key=?", c.Attempt, e.Key}}
		for _, b := range e.Blobs {
			stmts = append(stmts, []any{"INSERT INTO cache_entry_blob(cache_key,path,sha256,size) VALUES (?,?,?,?)", e.Key, b.Path, b.SHA256, b.Size})
		}
		return execAll(ctx, tx, stmts...)
	})
	return errors.Join(err, refusal)
}

type cacheQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// cacheScan runs q and calls each per row, closing the rows on every path.
func cacheScan(ctx context.Context, db cacheQuerier, q string, args []any, each func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		if err = each(rows); err != nil {
			break
		}
	}
	return errors.Join(err, rows.Err(), rows.Close())
}

// CacheEntries lists entries with their blobs; key "" lists all, oldest use first.
func (s *Store) CacheEntries(ctx context.Context, key string) ([]entries.Manifest, error) {
	var out []entries.Manifest
	err := cacheScan(ctx, s.readers, "SELECT cache_key,namespace,spec,attempt_id,epoch,source_trust,secrets_received,private_deps,scanned_clean,bytes,created_at,last_used_at FROM cache_entry WHERE ?='' OR cache_key=? ORDER BY last_used_at,cache_key", []any{key, key}, func(r *sql.Rows) error {
		var e entries.Manifest
		var bytes int64
		w := &e.Writer
		err := r.Scan(&e.Key, &e.Namespace, &e.Spec, &w.AttemptID, &w.Epoch, &w.SourceTrust, &w.SecretsReceived, &w.PrivateDeps, &w.ScannedClean, &bytes, &e.CreatedAt, &e.LastUsedAt)
		out = append(out, e)
		return err
	})
	for i := 0; err == nil && i < len(out); i++ {
		err = cacheScan(ctx, s.readers, "SELECT path,sha256,size FROM cache_entry_blob WHERE cache_key=? ORDER BY path", []any{out[i].Key}, func(r *sql.Rows) error {
			var b entries.Blob
			err := r.Scan(&b.Path, &b.SHA256, &b.Size)
			out[i].Blobs = append(out[i].Blobs, b)
			return err
		})
	}
	return out, err
}

// CacheTouch marks an entry used.
func (s *Store) CacheTouch(ctx context.Context, key string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		return execAll(ctx, tx, []any{"UPDATE cache_entry SET last_used_at=? WHERE cache_key=?", s.now().UnixNano(), key})
	})
}

// CacheDrop deletes the entry and appends the audit row in one transaction.
func (s *Store) CacheDrop(ctx context.Context, key, reason string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if err := execAll(ctx, tx, []any{"DELETE FROM cache_entry WHERE cache_key=?", key}); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, "cache", "cache.entry.dropped", key, "reason="+reason)
	})
}

// CachePrune runs one prune step in a single transaction (all or nothing).
func (s *Store) CachePrune(ctx context.Context, p entries.PruneStep) (entries.Pruned, error) {
	var out entries.Pruned
	err := s.write(ctx, func(tx *sql.Tx) error {
		out = entries.Pruned{}
		now := s.now()
		old, cutoff := now.Add(-p.Retention).UnixNano(), now.Add(-p.MaxAge).UnixNano()
		if err := execAll(ctx, tx, []any{"DELETE FROM cache_use WHERE ts<?", old}, []any{"DELETE FROM cache_attempt WHERE recorded_at<?", old}); err != nil {
			return err
		}
		type row struct {
			key        string
			bytes, use int64
		}
		var all []row
		var total int64
		err := cacheScan(ctx, tx, "SELECT cache_key,bytes,last_used_at FROM cache_entry ORDER BY last_used_at,cache_key", nil, func(r *sql.Rows) error {
			var x row
			err := r.Scan(&x.key, &x.bytes, &x.use)
			all, total = append(all, x), total+x.bytes
			return err
		})
		for _, r := range all {
			if err != nil || !(r.use < cutoff || total > p.MaxBytes || out.Bytes < p.FreeBytes) {
				break
			}
			if p.Limit > 0 && out.Entries >= p.Limit {
				out.More = true
				break
			}
			err = execAll(ctx, tx, []any{"DELETE FROM cache_entry WHERE cache_key=?", r.key})
			out.Entries, out.Bytes, total = out.Entries+1, out.Bytes+r.bytes, total-r.bytes
		}
		return err
	})
	return out, err
}

// CacheRecordUse appends a usage row (the table CHECK limits Result).
func (s *Store) CacheRecordUse(ctx context.Context, u entries.Use) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		return execAll(ctx, tx, []any{"INSERT INTO cache_use(attempt_id,spec,cache_key,result,bytes,ms,ts) VALUES (?,?,?,?,?,?,?)", u.Attempt, u.Spec, u.Key, u.Result, u.Bytes, u.MS, s.now().UnixNano()})
	})
}

// CacheUses lists the usage rows of one attempt, oldest first.
func (s *Store) CacheUses(ctx context.Context, attempt string) ([]entries.Use, error) {
	var out []entries.Use
	err := cacheScan(ctx, s.readers, "SELECT attempt_id,spec,cache_key,result,bytes,ms,ts FROM cache_use WHERE attempt_id=? ORDER BY id", []any{attempt}, func(r *sql.Rows) error {
		var u entries.Use
		err := r.Scan(&u.Attempt, &u.Spec, &u.Key, &u.Result, &u.Bytes, &u.MS, &u.TS)
		out = append(out, u)
		return err
	})
	return out, err
}
