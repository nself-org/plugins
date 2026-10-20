package audit

// Purpose: writing and sealing the chained audit log (Epic D20) over the core audit table.
// Inputs:  Event values; the store; an optional Scanner.
// Outputs: one sealed row per Append; Seal chains rows other packages wrote through the CI-31 API.
// Constraints: Append scrubs first, then inserts and seals every pending row in one BEGIN IMMEDIATE
//          transaction (the store writer connection), so concurrent appenders and processes lose no row;
//          AppendTx joins the caller's transaction for P7-TRUST-26 policy writes; the secret check runs
//          before any write.

import (
	"context"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Event is one audit row. Detail holds references and decision metadata only.
type Event struct{ Actor, Action, Target, Detail string }

// Log writes to one store.
type Log struct {
	St    *store.Store
	Scrub Scanner
}

func (e Event) input() store.AuditInput {
	return store.AuditInput{Actor: e.Actor, Action: e.Action, Target: e.Target, Detail: e.Detail, Seal: Chain}
}

// Append writes ev and seals the log up to it.
func (l *Log) Append(ctx context.Context, ev Event) error {
	if err := Scrub(l.Scrub, ev); err != nil {
		return err
	}
	return l.St.AuditAppend(ctx, ev.input())
}

// Input is the audit input of ev for a store mutation that writes its own row in the same transaction.
func (l *Log) Input(ev Event) (store.AuditInput, error) {
	if err := Scrub(l.Scrub, ev); err != nil {
		return store.AuditInput{}, err
	}
	return ev.input(), nil
}

// Seal chains every unsealed row and returns how many it sealed.
func (l *Log) Seal(ctx context.Context) (int, error) { return l.St.AuditSeal(ctx, Chain) }

// AcceptBreak records that actor inspected the cleared head at seq (the first row `Verify` reports) and
// re-chains from it; see store.AuditAcceptBreak. It returns how many rows it re-chained.
func (l *Log) AcceptBreak(ctx context.Context, seq int64, actor string) (int, error) {
	in, err := l.Input(Event{Actor: actor, Action: "audit.accept-break", Target: "audit", Detail: "seq=" + strconv.FormatInt(seq, 10)})
	if err != nil {
		return 0, err
	}
	return l.St.AuditAcceptBreak(ctx, Chain, seq, in)
}

// AppendTx writes ev inside tx (a policy write's transaction); the process-wide scanner applies.
func AppendTx(tx *store.Tx, ev Event) error {
	if err := Scrub(nil, ev); err != nil {
		return err
	}
	return tx.AuditAppend(ev.input())
}

// RunSealer seals the log every interval (default SealInterval) until ctx ends: the TEAM coordinator's
// sealer for rows other packages write through the core audit API. A failed pass is passed to onErr
// (may be nil) and the next tick tries again.
func (l *Log) RunSealer(ctx context.Context, every time.Duration, onErr func(error)) {
	if every <= 0 {
		every = SealInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := l.Seal(ctx); err != nil && onErr != nil && ctx.Err() == nil {
				onErr(err)
			}
		}
	}
}
