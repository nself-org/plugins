package run

// Purpose: restart recovery, limited to stale coordinators. A coordinator
// whose process is gone (same boot, pid dead) or whose heartbeat expired is
// stale; its running attempts become lost, then retryable, or needs-operator
// for a job that must not run twice; its orphan processes and containers are
// swept. A live coordinator is never touched.
// Inputs:  the store, a sweeper executor and this boot. Outputs: a report.
// Constraints: every step is idempotent; a failure of one step is reported
// and the rest still run.

import (
	"context"
	"errors"
	"os"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RecoverReport says what recovery changed.
type RecoverReport struct {
	Expired, Lost, Retryable, NeedsOperator int
	Errors                                  []error
}

// staleChecker answers the executor's orphan sweep from the store.
type staleChecker struct {
	st   *store.Store
	boot string
}

// Stale reports whether the coordinator with this (safe) id is gone.
func (c staleChecker) Stale(ctx context.Context, id string) (bool, error) {
	all, err := c.st.Coordinators(ctx)
	if err != nil {
		return false, err
	}
	stale, err := c.st.StaleCoordinators(ctx, c.boot)
	if err != nil {
		return false, err
	}
	for _, s := range stale {
		if safeID(s.ID) == id {
			return true, nil
		}
	}
	for _, a := range all {
		if safeID(a.ID) == id {
			return false, nil
		}
	}
	return true, nil // no such coordinator: its leftovers belong to nobody
}

// Recover cleans up after coordinators that died. current is the live
// coordinator id of the caller ("" for a read-only caller such as `ci runs`);
// home is the state directory of the caller ("" = NSELF_CI_HOME or the default).
func Recover(ctx context.Context, st *store.Store, current, home string) RecoverReport {
	var rep RecoverReport
	boot := bootID()
	all, err := st.Coordinators(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return rep
	}
	for _, c := range all {
		if c.ID != current && c.BootID == boot && !pidAlive(c.PID) {
			if err = st.ExpireCoordinator(ctx, c.ID); err != nil {
				rep.Errors = append(rep.Errors, err)
				continue
			}
			rep.Expired++
		}
	}
	sweeper := ciexec.Executor{Stale: staleChecker{st, boot}}
	// Sweep reads NSELF_CI_HOME; never sweep a home other than the run's own
	// (the executor sweeps the right one when the next job starts).
	if home != "" && home != os.Getenv("NSELF_CI_HOME") {
		sweeper.Stale = nil
	}
	if err = sweeper.Sweep(ctx, safeID(current)); err != nil {
		rep.Errors = append(rep.Errors, err)
	}
	n, err := st.SweepStale(ctx, boot)
	rep.Lost = n
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return rep
	}
	lost, err := st.LostAttempts(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return rep
	}
	for _, a := range lost {
		d := store.TransitionDetail{Epoch: a.Epoch, FailureClass: "lost", Reason: "coordinator lost"}
		err = st.Transition(ctx, a.ID, "lost", "retryable", d)
		var se *store.Error
		switch {
		case err == nil:
			rep.Retryable++
		case errors.As(err, &se) && (se.Code == "E604" || se.Code == "E609"):
			if err = st.Transition(ctx, a.ID, "lost", "needs-operator", d); err != nil {
				rep.Errors = append(rep.Errors, err)
				continue
			}
			rep.NeedsOperator++
		default:
			rep.Errors = append(rep.Errors, err)
		}
	}
	return rep
}
