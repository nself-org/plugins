package store

// Purpose: the durable intake of the V15 coordinator (P7-NODE-10, Epic D7): one row per accepted trigger,
// the delivery ids seen, and the idempotency keys of manual triggers. A trigger is stored before the
// caller is told it was accepted, so a crash between accept and dispatch loses nothing.
// Inputs:  IntakeRequest (the trigger with its semantic key), or a claim owner for the dispatch loop.
// Outputs: IntakeOutcome (enqueued, duplicate, coalesced or replay), claimed rows, queue statistics.
// Constraints: a delivery id (source + id) runs once; a semantic key coalesces into a live row, and for a
//          webhook-sourced trigger also into a row completed within the horizon (72 h, the provider
//          redelivery window) unless Force, which writes an audit row; the idempotency key is stored as a
//          digest only; claims carry a heartbeat time so a dead owner's rows return to the queue.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"time"
)

//go:embed migrations/node-intake/*.sql
var intakeMigrations embed.FS

func init() {
	if err := Register("node-intake", intakeMigrations); err != nil {
		panic(err)
	}
}

// Intake states and enqueue outcomes.
const (
	IntakeQueued     = "queued"
	IntakeDispatched = "dispatched"
	IntakeDone       = "done"
	IntakeFailed     = "failed"

	IntakeEnqueued  = "enqueued"
	IntakeDuplicate = "duplicate"
	IntakeCoalesced = "coalesced"
	IntakeReplay    = "replay"

	// IntakeRetention is the provider redelivery horizon: deliveries, completed rows and keys older than
	// this are forgotten by DedupPrune.
	IntakeRetention = 72 * time.Hour
)

// IntakeRow is one accepted trigger.
type IntakeRow struct {
	Seq                                                                   int64
	ID, Source, DeliveryID, Project, Revision, TriggerClass, PolicyDigest string
	SemanticKey, Pipeline                                                 string
	Facts                                                                 []byte
	Webhook, Forced                                                       bool
	State, ClaimedBy, PipelineID, Result                                  string
	Attempts                                                              int
	CreatedAt, FinishedAt                                                 int64
}

// IntakeRequest is one trigger to store. Horizon 0 means IntakeRetention.
type IntakeRequest struct {
	Source, DeliveryID, Project, Revision, TriggerClass, PolicyDigest, SemanticKey, Pipeline string
	Facts                                                                                    []byte
	Webhook, Force                                                                           bool
	Actor                                                                                    string
	Horizon                                                                                  time.Duration
	IdemDigest, ReqDigest                                                                    string
}

// IntakeOutcome says what IntakeEnqueue did. ID is the row the trigger now stands for (the existing one
// for duplicate, coalesced and replay).
type IntakeOutcome struct{ Kind, ID string }

// IntakeStats are the counts /healthz shows.
type IntakeStats struct{ Queued, Dispatched, Leases int }

const intakeCols = `seq,id,source,delivery_id,project,revision,trigger_class,policy_digest,semantic_key,pipeline,facts,webhook,forced,state,claimed_by,attempts,pipeline_id,result,created_at,finished_at`

func scanIntake(sc interface{ Scan(...any) error }) (IntakeRow, error) {
	var r IntakeRow
	err := sc.Scan(&r.Seq, &r.ID, &r.Source, &r.DeliveryID, &r.Project, &r.Revision, &r.TriggerClass, &r.PolicyDigest, &r.SemanticKey, &r.Pipeline,
		&r.Facts, &r.Webhook, &r.Forced, &r.State, &r.ClaimedBy, &r.Attempts, &r.PipelineID, &r.Result, &r.CreatedAt, &r.FinishedAt)
	return r, err
}

func newIntakeID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "in-" + hex.EncodeToString(b)
}

// IntakeEnqueue stores a trigger in one transaction with its delivery and key rows (and the audit row of
// a forced enqueue). The caller may answer 2xx once it returns without error.
func (s *Store) IntakeEnqueue(ctx context.Context, r IntakeRequest) (IntakeOutcome, error) {
	var out IntakeOutcome
	err := s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		var id string
		if r.IdemDigest != "" {
			var req string
			switch e := tx.QueryRowContext(ctx, `SELECT req_digest,intake_id FROM intake_idem WHERE key_digest=?`, r.IdemDigest).Scan(&req, &id); {
			case e == nil && req != r.ReqDigest:
				return coded("E608", "idempotency key already used for a different request")
			case e == nil:
				out = IntakeOutcome{IntakeReplay, id}
				return nil
			case !errors.Is(e, sql.ErrNoRows):
				return e
			}
		}
		if r.DeliveryID != "" {
			switch e := tx.QueryRowContext(ctx, `SELECT intake_id FROM intake_delivery WHERE source=? AND delivery_id=?`, r.Source, r.DeliveryID).Scan(&id); {
			case e == nil:
				out = IntakeOutcome{IntakeDuplicate, id}
				return nil
			case !errors.Is(e, sql.ErrNoRows):
				return e
			}
		}
		horizon := r.Horizon
		if horizon <= 0 {
			horizon = IntakeRetention
		}
		var into string
		e := tx.QueryRowContext(ctx, `SELECT id FROM intake WHERE semantic_key=? AND (state IN ('queued','dispatched') OR (?=1 AND state='done' AND finished_at>?)) ORDER BY seq DESC LIMIT 1`,
			r.SemanticKey, r.Webhook, now-int64(horizon)).Scan(&into)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		id, kind := newIntakeID(), IntakeEnqueued
		if into != "" && !r.Force {
			id, kind = into, IntakeCoalesced
		} else if into != "" {
			actor := r.Actor
			if actor == "" {
				actor = "coordinator"
			}
			if e = s.appendAudit(ctx, tx, actor, "intake.force", id, "semantic_key="+r.SemanticKey+" into="+into); e != nil {
				return e
			}
		}
		if kind == IntakeEnqueued {
			if _, e = tx.ExecContext(ctx, `INSERT INTO intake(id,source,delivery_id,project,revision,trigger_class,policy_digest,semantic_key,pipeline,facts,webhook,forced,state,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, r.Source, r.DeliveryID, r.Project, r.Revision, r.TriggerClass, r.PolicyDigest, r.SemanticKey, r.Pipeline, r.Facts, r.Webhook, into != "", IntakeQueued, now); e != nil {
				return e
			}
		}
		if r.DeliveryID != "" {
			if _, e = tx.ExecContext(ctx, `INSERT INTO intake_delivery(source,delivery_id,intake_id,outcome,created_at) VALUES (?,?,?,?,?)`, r.Source, r.DeliveryID, id, kind, now); e != nil {
				return e
			}
		}
		if r.IdemDigest != "" {
			if _, e = tx.ExecContext(ctx, `INSERT INTO intake_idem(key_digest,req_digest,intake_id,created_at) VALUES (?,?,?,?)`, r.IdemDigest, r.ReqDigest, id, now); e != nil {
				return e
			}
		}
		out = IntakeOutcome{kind, id}
		return nil
	})
	return out, err
}

// IntakeGet reads one row.
func (s *Store) IntakeGet(ctx context.Context, id string) (IntakeRow, error) {
	r, err := scanIntake(s.readers.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, coded("E605", "unknown intake row")
	}
	return r, err
}

// DedupLookup finds the row a trigger with this semantic key would coalesce into: a live one, or (when
// completed is positive) one completed within that window. The enqueue repeats the check inside its own
// transaction; this read serves reports and tests.
func (s *Store) DedupLookup(ctx context.Context, key string, completed time.Duration) (IntakeRow, bool, error) {
	r, err := scanIntake(s.readers.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE semantic_key=? AND (state IN ('queued','dispatched') OR (?>0 AND state='done' AND finished_at>?)) ORDER BY seq DESC LIMIT 1`,
		key, int64(completed), s.now().UnixNano()-int64(completed)))
	if errors.Is(err, sql.ErrNoRows) {
		return IntakeRow{}, false, nil
	}
	return r, err == nil, err
}

// DedupPrune forgets finished rows, delivery ids and keys older than retention (0 means IntakeRetention)
// and returns how many rows it removed.
func (s *Store) DedupPrune(ctx context.Context, retention time.Duration) (int, error) {
	if retention <= 0 {
		retention = IntakeRetention
	}
	cut, n := s.now().UnixNano()-int64(retention), 0
	err := s.write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM intake WHERE state IN ('done','failed') AND finished_at<?`,
			`DELETE FROM intake_delivery WHERE created_at<?`,
			`DELETE FROM intake_idem WHERE created_at<?`,
		} {
			res, err := tx.ExecContext(ctx, q, cut)
			if err != nil {
				return err
			}
			c, _ := res.RowsAffected()
			n += int(c)
		}
		return nil
	})
	return n, err
}

// IntakeClaim hands the oldest queued row to owner, or reports false when the queue is empty.
func (s *Store) IntakeClaim(ctx context.Context, owner string) (IntakeRow, bool, error) {
	var r IntakeRow
	ok := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		row, err := scanIntake(tx.QueryRowContext(ctx, `SELECT `+intakeCols+` FROM intake WHERE state='queued' ORDER BY seq LIMIT 1`))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		now := s.now().UnixNano()
		if _, err = tx.ExecContext(ctx, `UPDATE intake SET state='dispatched',claimed_by=?,claim_at=?,attempts=attempts+1 WHERE id=? AND state='queued'`, owner, now, row.ID); err != nil {
			return err
		}
		row.State, row.ClaimedBy, row.Attempts = IntakeDispatched, owner, row.Attempts+1
		r, ok = row, true
		return nil
	})
	return r, ok, err
}

// IntakeBeat refreshes the claim time of every row owner is dispatching.
func (s *Store) IntakeBeat(ctx context.Context, owner string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE intake SET claim_at=? WHERE state='dispatched' AND claimed_by=?`, s.now().UnixNano(), owner)
		return err
	})
}

// IntakeFinish ends a claimed row: done with the pipeline run id, or failed with the reason.
func (s *Store) IntakeFinish(ctx context.Context, id, owner, pipelineID, result string, ok bool) error {
	state := IntakeFailed
	if ok {
		state = IntakeDone
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE intake SET state=?,pipeline_id=?,result=?,finished_at=? WHERE id=? AND state='dispatched' AND claimed_by=?`,
			state, pipelineID, result, s.now().UnixNano(), id, owner)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return coded("E605", "intake row is no longer claimed by this coordinator")
		}
		return nil
	})
}

// IntakeRequeue gives a claimed row back to the queue (a coordinator that is shutting down).
func (s *Store) IntakeRequeue(ctx context.Context, id, owner string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE intake SET state='queued',claimed_by='' WHERE id=? AND state='dispatched' AND claimed_by=?`, id, owner)
		return err
	})
}

// IntakeRecover returns to the queue every row another owner claimed and has not refreshed within stale.
func (s *Store) IntakeRecover(ctx context.Context, owner string, stale time.Duration) (int, error) {
	n := 0
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE intake SET state='queued',claimed_by='' WHERE state='dispatched' AND claimed_by<>? AND claim_at<=?`, owner, s.now().UnixNano()-int64(stale))
		if err != nil {
			return err
		}
		c, _ := res.RowsAffected()
		n = int(c)
		return nil
	})
	return n, err
}

// IntakeStats counts queued and dispatched rows and the leases that have not expired.
func (s *Store) IntakeStats(ctx context.Context) (IntakeStats, error) {
	var st IntakeStats
	err := s.readers.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM intake WHERE state='queued'),
 (SELECT count(*) FROM intake WHERE state='dispatched'),
 (SELECT count(*) FROM lease WHERE expires_at>?)`, s.now().UnixNano()).Scan(&st.Queued, &st.Dispatched, &st.Leases)
	return st, err
}
