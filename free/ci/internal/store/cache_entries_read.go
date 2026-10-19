package store

// Purpose: SQL for the cache entries (P7-CACHE-01), read side: entry reads in
// one statement (so an entry's facts and blobs always come from one snapshot),
// touch, drop, one LRU prune step and the usage rows.
// Inputs: keys, entries.PruneStep, entries.Use. Outputs: entries.Manifest values.
// Constraints: no env or secret value is ever stored; every function here is a
// single statement or a single transaction.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

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

// CacheEntries lists entries with their blobs from one snapshot; key "" lists
// all, least recently used first.
func (s *Store) CacheEntries(ctx context.Context, key string) ([]entries.Manifest, error) {
	var out []entries.Manifest
	q, args := `SELECT e.cache_key,e.namespace,e.spec,e.attempt_id,e.epoch,e.source_trust,e.secrets_received,e.private_deps,e.scanned_clean,e.created_at,e.last_used_at,b.path,b.sha256,b.size
 FROM cache_entry e LEFT JOIN cache_entry_blob b ON b.namespace=e.namespace AND b.cache_key=e.cache_key`, []any(nil)
	if key != "" {
		q, args = q+" WHERE e.cache_key=?", []any{key}
	}
	err := cacheScan(ctx, s.readers, q+" ORDER BY e.last_used_at,e.cache_key,e.namespace,b.path", args, func(r *sql.Rows) error {
		var e entries.Manifest
		var path, sha sql.NullString
		var size sql.NullInt64
		w := &e.Writer
		if err := r.Scan(&e.Key, &e.Namespace, &e.Spec, &w.AttemptID, &w.Epoch, &w.SourceTrust, &w.SecretsReceived, &w.PrivateDeps, &w.ScannedClean, &e.CreatedAt, &e.LastUsedAt, &path, &sha, &size); err != nil {
			return err
		}
		if n := len(out); n == 0 || out[n-1].Key != e.Key || !bytes.Equal(out[n-1].Namespace, e.Namespace) {
			out = append(out, e)
		}
		if path.Valid {
			last := &out[len(out)-1]
			last.Blobs = append(last.Blobs, entries.Blob{Path: path.String, SHA256: sha.String, Size: size.Int64})
		}
		return nil
	})
	return out, err
}

// CacheTouch marks an entry used.
func (s *Store) CacheTouch(ctx context.Context, ns []byte, key string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		return execAll(ctx, tx, []any{"UPDATE cache_entry SET last_used_at=? WHERE namespace=? AND cache_key=?", s.now().UnixNano(), ns, key})
	})
}

// CacheDrop deletes the entry m (only if it is still the row that was read: same
// writer attempt, epoch and creation time) and appends the audit row in one
// transaction.
func (s *Store) CacheDrop(ctx context.Context, m entries.Manifest, reason string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if err := execAll(ctx, tx, []any{"DELETE FROM cache_entry WHERE namespace=? AND cache_key=? AND attempt_id=? AND epoch=? AND created_at=?", m.Namespace, m.Key, m.Writer.AttemptID, m.Writer.Epoch, m.CreatedAt}); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, "cache", "cache.entry.dropped", m.Key, "reason="+reason)
	})
}

// CachePrune runs one prune step in a single transaction (all or nothing):
// usage rows and facts of attempts that are not running or finalizing, older than Retention, entries unused
// since MaxAge, then oldest first until total bytes fit MaxBytes and FreeBytes
// were released, at most Limit entries (0 = all); More reports that Limit
// stopped it with work left.
func (s *Store) CachePrune(ctx context.Context, p entries.PruneStep) (entries.Pruned, error) {
	var out entries.Pruned
	err := s.write(ctx, func(tx *sql.Tx) error {
		out = entries.Pruned{}
		now := s.now()
		old, cutoff := now.Add(-p.Retention).UnixNano(), now.Add(-p.MaxAge).UnixNano()
		if err := execAll(ctx, tx, []any{"DELETE FROM cache_use WHERE ts<?", old}, []any{"DELETE FROM cache_attempt WHERE recorded_at<? AND NOT EXISTS (SELECT 1 FROM attempt a WHERE a.id=cache_attempt.attempt_id AND a.state IN ('running','finalizing'))", old}); err != nil {
			return err
		}
		type row struct {
			key        string
			ns         []byte
			bytes, use int64
		}
		var all []row
		var total int64
		err := cacheScan(ctx, tx, "SELECT cache_key,namespace,bytes,last_used_at FROM cache_entry ORDER BY last_used_at,cache_key,namespace", nil, func(r *sql.Rows) error {
			var x row
			err := r.Scan(&x.key, &x.ns, &x.bytes, &x.use)
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
			err = execAll(ctx, tx, []any{"DELETE FROM cache_entry WHERE namespace=? AND cache_key=?", r.ns, r.key})
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
