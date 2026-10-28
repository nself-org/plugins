package run

// Purpose: restart recovery, limited to stale coordinators. A coordinator
// whose process is gone (same boot, pid dead) or whose heartbeat expired is
// stale; its running attempts become lost, then retryable, or needs-operator
// for a job that must not run twice; its orphan processes and containers are
// swept. A live coordinator is never touched.
// Inputs:  the store, a sweeper executor and this boot. Outputs: a report.
// Liveness is a pid check on the same boot id, so coordinators sharing one
// state directory must share one pid namespace (one host, no container split).
// Constraints: every step is idempotent; a failure of one step is reported
// and the rest still run.

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RecoverReport says what recovery changed.
type RecoverReport struct {
	Expired, Lost, Retryable, NeedsOperator, Stranded int
	Errors                                            []error
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
	return true, nil // no such coordinator: its own-home leftovers (process markers) belong to nobody; containers are gated by Known
}

// Known reports whether the store has a coordinator with this (safe) id. The
// container sweep lists every labelled container on the daemon, so an id this
// store has never seen may be a live coordinator of another CI home.
func (c staleChecker) Known(ctx context.Context, id string) (bool, error) {
	all, err := c.st.Coordinators(ctx)
	if err != nil {
		return false, err
	}
	for _, a := range all {
		if safeID(a.ID) == id {
			return true, nil
		}
	}
	return false, nil
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
	if home != "" && filepath.Clean(home) != filepath.Clean(os.Getenv("NSELF_CI_HOME")) {
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
	rep.Stranded = strandAttempts(ctx, st, boot, &rep)
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
		case CodeOf(err) == codeStoreRef: // a cancel intent won: the operator's cancel ends the lost attempt
			if err = st.Transition(ctx, a.ID, "lost", "cancelled", store.TransitionDetail{Epoch: a.Epoch, FailureClass: "cancelled", Reason: "coordinator lost"}); err != nil {
				rep.Errors = append(rep.Errors, err)
			}
		case errors.As(err, &se) && (se.Code == "E604" || se.Code == "E609"):
			err = st.Transition(ctx, a.ID, "lost", "needs-operator", d)
			if CodeOf(err) == codeStoreRef { // same, for a job that must not run twice
				err = st.Transition(ctx, a.ID, "lost", "cancelled", store.TransitionDetail{Epoch: a.Epoch, FailureClass: "cancelled", Reason: "coordinator lost"})
			}
			if err != nil {
				rep.Errors = append(rep.Errors, err)
				continue
			}
			rep.NeedsOperator++
		default:
			rep.Errors = append(rep.Errors, err)
		}
	}
	// a run whose coordinator died stays "running" until closed; one
	// transaction selects and closes it, and a recovery that was itself
	// killed earlier is finished here
	if _, err = st.CloseDeadRuns(ctx, boot); err != nil {
		rep.Errors = append(rep.Errors, err)
	}
	settleDeadRuns(ctx, st, &rep)
	return rep
}

// strandAttempts ends the queued and finalizing attempts of dead coordinators
// (queued: cancelled; finalizing: failed, class infra).
func strandAttempts(ctx context.Context, st *store.Store, boot string, rep *RecoverReport) int {
	list, err := st.StrandedAttempts(ctx, boot)
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return 0
	}
	n := 0
	for _, a := range list {
		to, class := model.JobState("cancelled"), model.FailureClass("cancelled")
		if a.State == "finalizing" {
			to, class = "failed", "infra"
		}
		d := store.TransitionDetail{Epoch: a.Epoch, FailureClass: class, Reason: "coordinator lost"}
		err = st.Transition(ctx, a.ID, model.JobState(a.State), to, d)
		if err != nil && CodeOf(err) == codeStoreRef && to != "cancelled" { // a cancel intent won
			err = st.Transition(ctx, a.ID, model.JobState(a.State), "cancelled", store.TransitionDetail{Epoch: a.Epoch, FailureClass: "cancelled", Reason: "coordinator lost"})
		}
		if err != nil {
			rep.Errors = append(rep.Errors, err)
			continue
		}
		n++
	}
	return n
}

// RecoveryFinalizer settles a dead run without relying on the crashed process's
// context or invoking publication and notification hooks.
type RecoveryFinalizer interface {
	RecoverRun(context.Context, *store.Store, string) error
}

func settleDeadRuns(ctx context.Context, st *store.Store, rep *RecoverReport) {
	ids, err := st.UnsettledDeadRuns(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return
	}
	for _, id := range ids {
		for _, f := range Registered.Finalizers {
			if recovery, ok := f.(RecoveryFinalizer); ok {
				if err := recovery.RecoverRun(ctx, st, id); err != nil {
					rep.Errors = append(rep.Errors, err)
				}
			}
		}
	}
}
