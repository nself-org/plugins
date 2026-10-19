package store

// Purpose: the durable intake of the V15 coordinator (P7-NODE-10, Epic D7): one row per accepted trigger,
// the delivery ids seen, and the idempotency keys of manual triggers. A trigger is stored before the
// caller is told it was accepted, so a crash between accept and dispatch loses nothing.
// Inputs:  IntakeRequest (the trigger with its semantic key), or a claim owner for the dispatch loop.
// Outputs: IntakeOutcome (enqueued, duplicate, coalesced or replay), claimed rows, queue statistics.
// Constraints: a delivery id runs once; a semantic key coalesces into a live row (a webhook trigger also into
//          a row completed within the 72 h redelivery horizon) unless Force, which writes an audit row; the
//          idempotency key is stored as a digest; claims carry a heartbeat so a dead owner's rows requeue.

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

	// IntakeMaxAttempts is how often a row may be claimed before recovery fails it instead of requeueing.
	IntakeMaxAttempts = 3
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
	Source, Event, DeliveryID, Project, Revision, TriggerClass, PolicyDigest, SemanticKey, Pipeline string
	Facts                                                                                           []byte
	Webhook, Force                                                                                  bool
	Actor                                                                                           string
	Horizon                                                                                         time.Duration
	IdemDigest, ReqDigest                                                                           string
}

// IntakeOutcome says what IntakeEnqueue did; ID is the row the trigger now stands for.
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
			var st sql.NullString
			switch e := tx.QueryRowContext(ctx, `SELECT d.intake_id,i.state FROM intake_delivery d LEFT JOIN intake i ON i.id=d.intake_id WHERE d.source=? AND d.delivery_id=?`, r.Source, r.DeliveryID).Scan(&id, &st); {
			case e == nil && st.String != IntakeFailed: // a failed run may be redelivered
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
		e := tx.QueryRowContext(ctx, `SELECT id FROM intake WHERE semantic_key=? AND (state IN ('queued','dispatched') OR (?=1 AND state='done' AND finished_at>? AND source=? AND event=?)) ORDER BY seq DESC LIMIT 1`,
			r.SemanticKey, r.Webhook, now-int64(horizon), r.Source, r.Event).Scan(&into)
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
			if _, e = tx.ExecContext(ctx, `INSERT INTO intake(id,source,event,delivery_id,project,revision,trigger_class,policy_digest,semantic_key,pipeline,facts,webhook,forced,state,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, r.Source, r.Event, r.DeliveryID, r.Project, r.Revision, r.TriggerClass, r.PolicyDigest, r.SemanticKey, r.Pipeline, r.Facts, r.Webhook, into != "", IntakeQueued, now); e != nil {
				return e
			}
		}
		if r.DeliveryID != "" {
			if _, e = tx.ExecContext(ctx, `INSERT OR REPLACE INTO intake_delivery(source,delivery_id,intake_id,outcome,created_at) VALUES (?,?,?,?,?)`, r.Source, r.DeliveryID, id, kind, now); e != nil {
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
