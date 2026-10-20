package store

// Purpose: the notifier's durable state (P7-AUTO-18). Migration set notify. SQL lives only
// here; internal/notify reaches it through an interface.
//   notify_sent   one row per (repo, sha, context, kind): the de-duplication key, so one
//                 alert goes out per commit and a rerun with the same result stays quiet
//   notify_last   the last red or green result per (repo, ref, context): what makes a
//                 recovery message exact (it fires on red -> green only)
//   notify_digest one row per day a digest was sent
//   notify_sink   delivered and failed counts per sink, shown by `ci doctor --selftest`
// Inputs: ids, results and counts; never log text, URLs or argv (a sink name is the only
// sink identity stored: URLs and argv may carry tokens).
// Outputs: claims (true when this caller owns the send), the previous result, counters.
// Constraints: every write is one transaction on the single writer connection.

import (
	"context"
	"database/sql"
	"embed"
	"time"
)

//go:embed migrations/notify/*.sql
var notifyMigrations embed.FS

func init() {
	if err := Register("notify", notifyMigrations); err != nil {
		panic(err)
	}
}

// notifyRetention is how long a de-duplication or last-result row is kept: commits and
// branches come from untrusted pushes, so the tables must not grow without bound.
const notifyRetention = 90 * 24 * time.Hour

// NotifyClaim takes the right to send one (repo, sha, context, kind) message. It
// reports false when that message was already claimed.
func (s *Store) NotifyClaim(ctx context.Context, repo, sha, statusContext, kind string) (bool, error) {
	var won bool
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM notify_sent WHERE created_at<?", s.now().Add(-notifyRetention).UnixNano()); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO notify_sent(repo,sha,context,kind,created_at) VALUES (?,?,?,?,?)", repo, sha, statusContext, kind, s.now().UnixNano())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		won = n == 1
		return err
	})
	return won, err
}

// NotifyRelease gives a claim back, so a later rerun may send again (used when no sink took
// the message).
func (s *Store) NotifyRelease(ctx context.Context, repo, sha, statusContext, kind string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM notify_sent WHERE repo=? AND sha=? AND context=? AND kind=?", repo, sha, statusContext, kind)
		return err
	})
}

// NotifyObserve records result as the latest for (repo, ref, context) and returns the one it
// replaced ("" when there was none). Read and write are one transaction.
func (s *Store) NotifyObserve(ctx context.Context, repo, ref, statusContext, sha, result string) (string, error) {
	var prev string
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM notify_last WHERE updated_at<?", s.now().Add(-notifyRetention).UnixNano()); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT result FROM notify_last WHERE repo=? AND ref=? AND context=?", repo, ref, statusContext).Scan(&prev)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO notify_last(repo,ref,context,result,sha,updated_at) VALUES (?,?,?,?,?,?) ON CONFLICT(repo,ref,context) DO UPDATE SET result=excluded.result, sha=excluded.sha, updated_at=excluded.updated_at", repo, ref, statusContext, result, sha, s.now().UnixNano())
		return err
	})
	return prev, err
}

// NotifyDigestClaim takes the digest of one day (any stable day label). It reports false
// when that day's digest was already claimed.
func (s *Store) NotifyDigestClaim(ctx context.Context, day string) (bool, error) {
	var won bool
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO notify_digest(day,sent_at) VALUES (?,?)", day, s.now().UnixNano())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		won = n == 1
		return err
	})
	return won, err
}

// NotifyDigestRelease gives a day's digest claim back (no sink took it).
func (s *Store) NotifyDigestRelease(ctx context.Context, day string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM notify_digest WHERE day=?", day)
		return err
	})
}

// NotifyLastDigest returns when the latest digest was claimed (0 when none was).
func (s *Store) NotifyLastDigest(ctx context.Context) (int64, error) {
	var at sql.NullInt64
	if err := s.readers.QueryRowContext(ctx, "SELECT max(sent_at) FROM notify_digest").Scan(&at); err != nil {
		return 0, err
	}
	return at.Int64, nil
}

// DigestStats counts the runs that finished in a window, and the work still queued.
type DigestStats struct {
	Runs, Passed, Failed, Errored, Queued int
}

// NotifyDigestStats counts pipelines finished in (since, until] by status, and the jobs of
// unfinished runs that are waiting for a runner now (jobs of a crashed run do not count once
// its run is closed).
func (s *Store) NotifyDigestStats(ctx context.Context, since, until int64) (DigestStats, error) {
	var d DigestStats
	rows, err := s.readers.QueryContext(ctx, "SELECT status, count(*) FROM pipeline WHERE finished_at>? AND finished_at<=? GROUP BY status", since, until)
	if err != nil {
		return d, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var status string
		var n int
		if err = rows.Scan(&status, &n); err != nil {
			return d, err
		}
		d.Runs += n
		switch status {
		case "passed":
			d.Passed += n
		case "failed":
			d.Failed += n
		case "error":
			d.Errored += n
		}
	}
	if err = rows.Err(); err != nil {
		return d, err
	}
	err = s.readers.QueryRowContext(ctx, "SELECT count(*) FROM job j JOIN pipeline p ON p.id=j.pipeline_id WHERE j.status='queued' AND p.finished_at IS NULL").Scan(&d.Queued)
	return d, err
}

// NotifyRecordDelivery counts one delivery attempt of a sink.
func (s *Store) NotifyRecordDelivery(ctx context.Context, sink string, ok bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if ok {
			_, err := tx.ExecContext(ctx, "INSERT INTO notify_sink(sink,delivered,failed) VALUES (?,1,0) ON CONFLICT(sink) DO UPDATE SET delivered=delivered+1", sink)
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO notify_sink(sink,delivered,failed,last_failed_at) VALUES (?,0,1,?) ON CONFLICT(sink) DO UPDATE SET failed=failed+1, last_failed_at=excluded.last_failed_at", sink, s.now().UnixNano())
		return err
	})
}

// SinkStat is one sink's delivery counters.
type SinkStat struct {
	Sink              string
	Delivered, Failed int64
	LastFailedAt      int64 // 0 when it never failed
}

// NotifySinkStats lists the counters, sink name order.
func (s *Store) NotifySinkStats(ctx context.Context) ([]SinkStat, error) {
	return collect(ctx, s, func(r *sql.Rows) (v SinkStat, err error) {
		var failedAt sql.NullInt64
		err = r.Scan(&v.Sink, &v.Delivered, &v.Failed, &failedAt)
		v.LastFailedAt = failedAt.Int64
		return
	}, "SELECT sink,delivered,failed,last_failed_at FROM notify_sink ORDER BY sink")
}
