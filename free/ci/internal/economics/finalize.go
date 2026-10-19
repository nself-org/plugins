package economics

// Purpose: the P7-CI-37 Finalizer of the ledger (Epic D11, review F5). When a
// run closes it settles every attempt that holds a ledger row: the actual
// duration is priced with the rate the row was admitted under, and the
// reservations take the actual in the window they were made in.
// Inputs:  run.RunInfo (the coordinator's own per-job duration) and the attempt
// span the store holds.
// Outputs: act_ms, act_micro and the confidence on the ledger row; settled reservations.
// Constraints: a local actual is `computed` from the coordinator's timestamps;
// a remote attempt whose end is before its start, or a duration longer than
// the admitted bound, is clamped to the estimate and flagged `estimated`; a
// duration nobody recorded is `unknown` and settles at the estimate; the actual
// never exceeds the estimate; settling twice is a no-op; no policy is read, the
// row carries its own rate.

import (
	"context"
	"errors"
	"time"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type finalizer struct{}

func init() { cirun.RegisterFinalizer(finalizer{}) }

func (finalizer) Name() string { return "economics" }

func (finalizer) Finalize(ctx context.Context, info cirun.RunInfo) error {
	env, ok := envOf(ctx)
	if !ok {
		return nil
	}
	var errs []error
	for _, j := range info.Jobs {
		if j.AttemptID == "" {
			continue
		}
		row, ok, err := env.Store.LedgerAttempt(ctx, j.AttemptID)
		if err == nil && ok {
			err = settle(ctx, env.Store, row, j.Duration)
		}
		errs = append(errs, err)
	}
	// The run is closed: an attempt that is still open (an earlier try of a retried job, a
	// cancelled job) holds its worst case for nothing. It settles on the span the coordinator
	// recorded, or at its estimate (unknown) when there is none.
	open, err := env.Store.LedgerOpen(ctx, info.RunID)
	errs = append(errs, err)
	for _, row := range open {
		errs = append(errs, settle(ctx, env.Store, row, 0))
	}
	return errors.Join(errs...)
}

func settle(ctx context.Context, st *store.Store, row store.LedgerRow, observed time.Duration) error {
	if row.ActMicro != nil {
		return nil
	}
	start, end, err := st.AttemptSpan(ctx, row.Attempt)
	if err != nil {
		return err
	}
	ms, micro, conf := Actual(row, observed, start, end)
	_, err = st.LedgerSettle(ctx, row.Attempt, ms, micro, conf)
	return err
}

// Actual is the settled figure of an attempt: milliseconds, micro-units and confidence.
// observed is the coordinator's duration of the job; start and end are the attempt
// span in unix nanoseconds (zero when not recorded).
func Actual(row store.LedgerRow, observed time.Duration, start, end int64) (ms, micro int64, conf string) {
	priced := row.Class != ClassLocal && row.Class != ClassOwned
	estimate := func(c string) (int64, int64, string) { return row.EstMs, row.EstMicro, c }
	switch {
	case row.Remote && start > 0 && end > 0 && end < start:
		return estimate(ConfEstimated)
	case row.Remote && start > 0 && end >= start:
		ms = (end - start) / int64(time.Millisecond)
	case observed > 0:
		ms = observed.Milliseconds()
	case !row.Remote && start > 0 && end >= start:
		ms = (end - start) / int64(time.Millisecond)
	default:
		if !priced {
			return 0, 0, ConfUnknown
		}
		return estimate(ConfUnknown)
	}
	conf = ConfComputed
	if priced && row.EstMs > 0 && ms > row.EstMs {
		ms, conf = row.EstMs, ConfEstimated
	}
	if !priced {
		return ms, 0, conf
	}
	micro, _, overflow := Charge(row.Rate, ms)
	if overflow || micro > row.EstMicro {
		micro = row.EstMicro
	}
	return ms, micro, conf
}
