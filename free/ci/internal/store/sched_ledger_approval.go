package store

// Purpose: spend approvals of the usage ledger (P7-SCHED-07, Epic D11). An
// approval is bound to the run, its head SHA, its config digest and the
// estimate the approver saw; it stops counting the moment the pipeline row
// shows another SHA or digest, so a new push voids it without a write.
// Inputs:  calls from economics.ApproveSpend and the budget admission step.
// Outputs: PipelineFacts, pending estimates, the valid approvals.
// Constraints: the approval row and its audit row commit together; the pending
// estimate is the largest figure noted for the run and is never lowered by a
// later, smaller one.

import (
	"context"
	"database/sql"
	"errors"
)

// Approval is a recorded spend approval.
type Approval struct {
	Run, SHA, ConfigDigest string
	EstimateMicro          int64
	Actor                  string
	TS                     int64
}

// PipelineFacts are what the core pipeline row says about a run.
type PipelineFacts struct{ Project, Revision, ConfigDigest string }

// PipelineFacts reads the facts an approval binds to.
func (s *Store) PipelineFacts(ctx context.Context, run string) (PipelineFacts, bool, error) {
	var f PipelineFacts
	err := s.readers.QueryRowContext(ctx, "SELECT project,revision,input_digest FROM pipeline WHERE id=?", run).Scan(&f.Project, &f.Revision, &f.ConfigDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return PipelineFacts{}, false, nil
	}
	return f, err == nil, err
}

// ValidApproval is the estimate approved for run while its SHA and config digest are unchanged.
func (t LedgerTx) ValidApproval(run string) (int64, bool, error) {
	var est int64
	err := t.tx.QueryRowContext(t.ctx, "SELECT a.estimate_micro FROM approval a JOIN pipeline p ON p.id=a.run WHERE a.run=? AND a.sha=p.revision AND a.config_digest=p.input_digest", run).Scan(&est)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return est, err == nil, err
}

// ValidApprovals lists every run whose approval still matches its pipeline row (run to approved estimate).
func (s *Store) ValidApprovals(ctx context.Context) (map[string]int64, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT a.run,a.estimate_micro FROM approval a JOIN pipeline p ON p.id=a.run WHERE a.sha=p.revision AND a.config_digest=p.input_digest")
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var run string
		var est int64
		if err = rows.Scan(&run, &est); err != nil {
			return nil, coded("E607", err.Error())
		}
		out[run] = est
	}
	return out, rows.Err()
}

// Approvals lists the newest approvals, newest first.
func (s *Store) Approvals(ctx context.Context, limit int) ([]Approval, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT run,sha,config_digest,estimate_micro,actor,ts FROM approval ORDER BY ts DESC, run LIMIT ?", limit)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []Approval
	for rows.Next() {
		var a Approval
		if err = rows.Scan(&a.Run, &a.SHA, &a.ConfigDigest, &a.EstimateMicro, &a.Actor, &a.TS); err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PutApproval records an approval and its audit row in one commit.
func (s *Store) PutApproval(ctx context.Context, a Approval, auditDetail string) error {
	if a.Run == "" || a.Actor == "" || a.SHA == "" || a.EstimateMicro < 0 {
		return coded("E607", "an approval needs a run, a head SHA, an actor and an estimate")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		a.TS = s.now().UnixNano()
		if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO approval(run,sha,config_digest,estimate_micro,actor,ts) VALUES (?,?,?,?,?,?)", a.Run, a.SHA, a.ConfigDigest, a.EstimateMicro, a.Actor, a.TS); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a.Actor, "spend.approved", a.Run, auditDetail)
	})
}

// NotePending records the estimate of a run held for approval (the largest one wins).
func (s *Store) NotePending(ctx context.Context, run string, estimate int64) error {
	if run == "" || estimate < 0 {
		return coded("E607", "a pending estimate needs a run and a non-negative amount")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO approval_pending(run,estimate_micro,ts) VALUES (?,?,?)
 ON CONFLICT(run) DO UPDATE SET estimate_micro=MAX(estimate_micro,excluded.estimate_micro), ts=excluded.ts`, run, estimate, s.now().UnixNano())
		return err
	})
}

// PendingEstimate returns the figure noted for run and the details of its
// attempts' approval.pending refusals (the budget step writes the estimate into
// the refusal detail).
func (s *Store) PendingEstimate(ctx context.Context, run string) (noted int64, details []string, err error) {
	err = s.readers.QueryRowContext(ctx, "SELECT estimate_micro FROM approval_pending WHERE run=?", run).Scan(&noted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, nil, coded("E607", err.Error())
	}
	rows, err := s.readers.QueryContext(ctx, `SELECT r.detail FROM sched_refusal r JOIN attempt a ON a.id=r.attempt_id JOIN job j ON j.id=a.job_id WHERE j.pipeline_id=? AND r.reason='approval.pending'`, run)
	if err != nil {
		return 0, nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			return 0, nil, coded("E607", err.Error())
		}
		details = append(details, d)
	}
	return noted, details, rows.Err()
}
