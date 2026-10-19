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
	"sync"

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
	return true, nil // no such coordinator: its leftovers belong to nobody
}

// Recover cleans up after coordinators that died. current is the live
// coordinator id of the caller ("" for a read-only caller such as `ci runs`);
// home is the state directory of the caller ("" = NSELF_CI_HOME or the default).
func Recover(ctx context.Context, st *store.Store, current, home string) RecoverReport {
	rep, join := RecoverBeside(ctx, st, current, home)
	if err := join(); err != nil {
		rep.Errors = append(rep.Errors, err)
	}
	return rep
}

// RecoverBeside is Recover with the container sweep running beside the rest of
// the recovery (and beside whatever the caller does next): it asks the daemon,
// which can take longer than all the store work together. The returned function
// waits for that sweep and returns its error; call it before the store closes.
func RecoverBeside(ctx context.Context, st *store.Store, current, home string) (RecoverReport, func() error) {
	var rep RecoverReport
	boot := bootID()
	sweeper := ciexec.Executor{Stale: staleChecker{st, boot}}
	// Sweep reads NSELF_CI_HOME; never sweep a home other than the run's own
	// (the executor sweeps the right one when the next job starts).
	if home != "" && filepath.Clean(home) != filepath.Clean(os.Getenv("NSELF_CI_HOME")) {
		sweeper.Stale = nil
	}
	var containerErr error
	containerDone := make(chan struct{})
	go func() {
		defer close(containerDone)
		containerErr = sweeper.SweepContainers(ctx, safeID(current))
	}()
	join := sync.OnceValue(func() error { <-containerDone; return containerErr })
	all, err := st.Coordinators(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return rep, join
	}
	// A coordinator that is already expired (heartbeat 0) needs no write: rows are never removed,
	// so writing them again would add one transaction per past run to every later run.
	var dead []string
	for _, c := range all {
		if c.ID != current && c.BootID == boot && c.HeartbeatAt != 0 && !pidAlive(c.PID) {
			dead = append(dead, c.ID)
		}
	}
	if err = st.ExpireCoordinators(ctx, dead); err != nil {
		rep.Errors = append(rep.Errors, err)
	} else {
		rep.Expired = len(dead)
	}
	if err = sweeper.SweepProcesses(ctx, safeID(current)); err != nil {
		rep.Errors = append(rep.Errors, err)
	}
	n, err := st.SweepStale(ctx, boot)
	rep.Lost = n
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return rep, join
	}
	rep.Stranded = strandAttempts(ctx, st, boot, &rep)
	lost, err := st.LostAttempts(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err)
		return rep, join
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
	// a run whose coordinator died stays "running" until closed; one
	// transaction selects and closes it, and a recovery that was itself
	// killed earlier is finished here
	if _, err = st.CloseDeadRuns(ctx, boot); err != nil {
		rep.Errors = append(rep.Errors, err)
	}
	return rep, join
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
