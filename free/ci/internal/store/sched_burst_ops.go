package store

// Purpose: the counters and guards around burst intents (P7-SCHED-08): the
// one-job-per-node record, the join breaker, the teardown-failure list, the
// caps' counters and the decision log.
// Inputs:  calls from internal/burst and its `ephemeral` admission step.
// Outputs: counts, flags and rows.
// Constraints: a node use row is written in the admission transaction, so it
// commits with the lease or not at all; a failed teardown is alerted once (the
// caller learns it from TeardownFailed) and blocks its provider until the
// resource is confirmed gone; the breaker counts consecutive join failures and
// opens for a fixed time; counts include only intents that still hold money.

import (
	"context"
	"database/sql"
	"errors"
)

// NodeUse records that the ephemeral node took a lease for attemptID. It returns
// false when the node already took one (the caller refuses the second lease).
func (t BurstTx) NodeUse(nodeID, intent, attemptID string, at int64) (first bool, err error) {
	res, err := t.tx.ExecContext(t.ctx, `INSERT OR IGNORE INTO burst_node_use (node_id,intent,attempt_id,at) VALUES (?,?,?,?)`, nodeID, intent, attemptID, at)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// NodeUsed reports whether the node has held a lease.
func (t BurstTx) NodeUsed(nodeID string) (bool, error) {
	return burstExists(t.ctx, t.tx, `SELECT 1 FROM burst_node_use WHERE node_id=?`, nodeID)
}

// IntentOfNode returns the non-terminal intent bound to a node.
func (t BurstTx) IntentOfNode(nodeID string) (BurstIntent, bool, error) {
	i, err := scanIntent(t.tx.QueryRowContext(t.ctx, `SELECT `+intentCols+` FROM burst_intent WHERE node_id=? AND state<>'terminal'`, nodeID))
	if errors.Is(err, sql.ErrNoRows) {
		return BurstIntent{}, false, nil
	}
	return i, err == nil, err
}

func burstExists(ctx context.Context, q rowQuerier, query string, args ...any) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, query, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func burstCount(ctx context.Context, q rowQuerier, query string, args ...any) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// BurstNodeBusy reports whether the node has an attempt that is leased, running or finalizing.
func (s *Store) BurstNodeBusy(ctx context.Context, nodeID string) (bool, error) {
	return burstExists(ctx, s.readers, `SELECT 1 FROM lease l JOIN attempt a ON a.id=l.attempt_id WHERE l.runner_id=? AND a.state IN ('leased','running','finalizing') LIMIT 1`, nodeID)
}

// BurstNodeUsed reports whether the node has held a lease.
func (s *Store) BurstNodeUsed(ctx context.Context, nodeID string) (bool, error) {
	return burstExists(ctx, s.readers, `SELECT 1 FROM burst_node_use WHERE node_id=?`, nodeID)
}

// BurstCounts are the live counters of one provider.
type BurstCounts struct {
	// InFlight are live intents whose node has not joined yet: capacity on its way.
	InFlight int
	// Open are intents that still hold money (live or tearing down).
	Open int
	// JobWindow are intents started for the job key at or after the window start.
	JobWindow int
	// TeardownFailed are failed teardowns not yet confirmed gone.
	TeardownFailed int
}

// BurstCountsOf reads the counters of a provider for a job key.
func (s *Store) BurstCountsOf(ctx context.Context, provider, jobKey string, windowStart int64) (BurstCounts, error) {
	return burstCounts(ctx, s.readers, provider, jobKey, windowStart)
}

// Counts reads the same counters inside the transaction, so the caps are re-checked at the commit.
func (t BurstTx) Counts(provider, jobKey string, windowStart int64) (BurstCounts, error) {
	return burstCounts(t.ctx, t.tx, provider, jobKey, windowStart)
}

func burstCounts(ctx context.Context, q rowQuerier, provider, jobKey string, windowStart int64) (BurstCounts, error) {
	var c BurstCounts
	var err error
	if c.InFlight, err = burstCount(ctx, q, `SELECT COUNT(*) FROM burst_intent WHERE provider=? AND state='live' AND node_id=''`, provider); err != nil {
		return c, err
	}
	if c.Open, err = burstCount(ctx, q, `SELECT COUNT(*) FROM burst_intent WHERE provider=? AND state<>'terminal'`, provider); err != nil {
		return c, err
	}
	if c.JobWindow, err = burstCount(ctx, q, `SELECT COUNT(*) FROM burst_intent WHERE job_key=? AND created_at>=?`, jobKey, windowStart); err != nil {
		return c, err
	}
	c.TeardownFailed, err = burstCount(ctx, q, `SELECT COUNT(*) FROM burst_teardown_failure WHERE provider=?`, provider)
	return c, err
}

// BurstPending lists provider and platform of live intents whose node has not joined (one entry per intent).
func (s *Store) BurstPending(ctx context.Context) ([][2]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT provider,platform FROM burst_intent WHERE state='live' AND node_id='' ORDER BY created_at, token`)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out [][2]string
	for rows.Next() {
		var p [2]string
		if err = rows.Scan(&p[0], &p[1]); err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// BreakerState is a provider's join-failure breaker.
type BreakerState struct {
	Failures  int
	OpenUntil int64
}

// Breaker reads the breaker of a provider (closed when there is no row).
func (s *Store) Breaker(ctx context.Context, provider string) (BreakerState, error) {
	var b BreakerState
	err := s.readers.QueryRowContext(ctx, `SELECT failures,open_until FROM burst_breaker WHERE provider=?`, provider).Scan(&b.Failures, &b.OpenUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return BreakerState{}, nil
	}
	return b, err
}

// BreakerFail counts one more consecutive join failure and opens the breaker at threshold for openMs.
func (t BurstTx) BreakerFail(provider string, threshold int, openMs, now int64) (BreakerState, error) {
	if _, err := t.tx.ExecContext(t.ctx, `INSERT INTO burst_breaker (provider,failures,open_until) VALUES (?,1,0) ON CONFLICT(provider) DO UPDATE SET failures=failures+1`, provider); err != nil {
		return BreakerState{}, err
	}
	var b BreakerState
	if err := t.tx.QueryRowContext(t.ctx, `SELECT failures,open_until FROM burst_breaker WHERE provider=?`, provider).Scan(&b.Failures, &b.OpenUntil); err != nil {
		return b, err
	}
	if b.Failures >= threshold {
		b.OpenUntil = now + openMs
		_, err := t.tx.ExecContext(t.ctx, `UPDATE burst_breaker SET open_until=? WHERE provider=?`, b.OpenUntil, provider)
		return b, err
	}
	return b, nil
}

// BreakerReset closes the breaker after a join succeeded.
func (t BurstTx) BreakerReset(provider string) error {
	_, err := t.tx.ExecContext(t.ctx, `DELETE FROM burst_breaker WHERE provider=?`, provider)
	return err
}

// TeardownFailed records a failed teardown attempt. alert is true only the first
// time, so the operator is told once however long the retries last.
func (t BurstTx) TeardownFailed(intent, provider string, at int64) (alert bool, err error) {
	if _, err = t.tx.ExecContext(t.ctx, `INSERT INTO burst_teardown_failure (intent,provider,first_failed_at,last_failed_at,attempts,alerted) VALUES (?,?,?,?,1,0)
 ON CONFLICT(intent) DO UPDATE SET last_failed_at=excluded.last_failed_at, attempts=attempts+1`, intent, provider, at, at); err != nil {
		return false, err
	}
	res, err := t.tx.ExecContext(t.ctx, `UPDATE burst_teardown_failure SET alerted=1 WHERE intent=? AND alerted=0`, intent)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// BurstTeardownFailure is when a teardown first failed and how often it was retried.
type BurstTeardownFailure struct {
	FirstFailedAt int64
	Attempts      int
	Alerted       bool
}

// TeardownFailure reads the failure record of an intent.
func (s *Store) TeardownFailure(ctx context.Context, intent string) (BurstTeardownFailure, bool, error) {
	var f BurstTeardownFailure
	var alerted int
	err := s.readers.QueryRowContext(ctx, `SELECT first_failed_at,attempts,alerted FROM burst_teardown_failure WHERE intent=?`, intent).Scan(&f.FirstFailedAt, &f.Attempts, &alerted)
	if errors.Is(err, sql.ErrNoRows) {
		return f, false, nil
	}
	f.Alerted = alerted == 1
	return f, err == nil, err
}

// PutDecision logs one burst decision with every input (a JSON document).
func (t BurstTx) PutDecision(at int64, provider, jobKey string, allowed bool, reason, input string) error {
	a := 0
	if allowed {
		a = 1
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO burst_decision (at,provider,job_key,allowed,reason,input) VALUES (?,?,?,?,?,?)`, at, provider, jobKey, a, reason, input)
	return err
}

// BurstDecision is a logged decision.
type BurstDecision struct {
	At                              int64
	Provider, JobKey, Reason, Input string
	Allowed                         bool
}

// BurstDecisions lists the logged decisions of a job key, oldest first.
func (s *Store) BurstDecisions(ctx context.Context, jobKey string) ([]BurstDecision, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT at,provider,job_key,allowed,reason,input FROM burst_decision WHERE job_key=? ORDER BY id`, jobKey)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []BurstDecision
	for rows.Next() {
		var d BurstDecision
		var a int
		if err = rows.Scan(&d.At, &d.Provider, &d.JobKey, &a, &d.Reason, &d.Input); err != nil {
			return nil, coded("E607", err.Error())
		}
		d.Allowed = a == 1
		out = append(out, d)
	}
	return out, rows.Err()
}
