package run

import (
	"context"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// Run executes the plan. A returned error is an infrastructure or usage
// failure (exit 2 or 1 by its code); a failed gate is a Result with Exit 1.
func (c *Coordinator) Run(ctx context.Context) (Result, error) {
	ctx = policy.WithBranchSnapshot(ctx)
	h := c.Hooks.Resolved()
	if err := c.Opts.Validate(h); err != nil {
		return Result{}, err
	}
	if c.Plan == nil {
		return Result{}, fail(CodeOptions, "no plan to run")
	}
	keep, err := subset(c.Plan, c.Opts.Job)
	if err != nil {
		return Result{}, err
	}
	if err = requireRunnable(c.Plan, keep); err != nil {
		return Result{}, err
	}
	if c.mayAskRuntime() { // a run that never leaves process isolation does not pay for the daemon's answer up front
		ciexec.WarmRuntimeInfo()
	}
	rev := GitRevision(ctx, c.Opts.Root, c.capture())
	rev.Dirty = rev.Dirty || c.ConfigOutside
	checkout, release, err := c.offload(ctx, h, rev) // a snapshot is what nodes check out; the run records rev
	if err != nil {
		return Result{}, err
	}
	defer release()
	rev.Dirty = rev.Dirty || checkout != rev.SHA // a run whose nodes test a snapshot is a dev run: never a status of record
	// the evidence's start reads git only: it runs beside admission, policy and the store (P7-CI-46)
	begun := make(chan any, 1)
	go func() { begun <- c.beginEvidence(ctx, h, rev) }()
	admitted, source, err := c.classify(ctx, h, rev)
	if err != nil {
		return Result{}, err
	}
	pol, err := c.policy(ctx, h, model.TrustClass(admitted))
	if err != nil {
		return Result{}, err
	}
	st, err := OpenStore(c.storePath())
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = st.Close() }()
	coord, err := st.Register(ctx, os.Getpid(), strconv.FormatInt(time.Now().UnixNano(), 10), bootID())
	if err != nil {
		return Result{}, err
	}
	recovered := Recover(ctx, st, coord.ID, c.Opts.Home)
	// status preparation can wait on another poster for minutes; it runs before the run
	// is persisted so a persisted run is never left without attempts or heartbeats
	post := c.prepareStatus(ctx, st, rev)
	_ = st.Heartbeat(ctx, coord.ID)
	runID := newRunID()
	defer waitCacheCleanups(runID) // the attempts' cache directories are gone when the run returns
	defer dispatchRelease(runID)   // also closes the dispatcher of a run that ends before AfterRun (D-1044)
	sess := newSession()
	mp := c.Plan.ModelPipeline()
	rows, err := c.persist(ctx, st, runID, rev, source, pol, sess, mp)
	if err != nil {
		return Result{}, err
	}
	c.exec = newExecutor(staleChecker{st, bootID()}, h.NewRedactor, h.Isolation)
	sess.Store, sess.Coordinator, sess.Exec, sess.RunID, sess.Revision, sess.Checkout = st, coord, c.exec, runID, rev.SHA, checkout
	sess.Host, sess.Home, sess.Admission = hostName(), c.Opts.Home, c.Admission
	sess.Admission.Force = sess.Admission.Force || c.Opts.Force
	sess.Plan, sess.Record = c.Plan, c.Opts.Record
	sess.PolicyRun, sess.Interactive, sess.ProviderRepo = c.Opts.PolicyRun, c.Opts.Interactive, c.Opts.ProviderRepo
	sess.User, sess.Team = c.Opts.User, c.Opts.Team
	sess.Placement, sess.Queue, sess.Spend = pol.Placement, pol.Queue, pol.Spend
	sess.Capacity = c.Capacity
	if sess.Capacity == nil {
		sess.Capacity = ciexec.Probe
	}
	if c.Poll > 0 {
		sess.Poll = c.Poll
	}
	out := c.Out
	if out == nil {
		out = io.Discard
	}
	subst := c.Substantive
	if subst == nil {
		subst = func(plan.PlannedJob) bool { return true }
	}
	rs := &runState{source: model.TrustClass(source), c: c, sess: sess, hooks: h, pol: pol, pl: c.Plan, mp: mp, rev: rev, rows: rows, subst: subst, render: stampRun(journal(ctx, st, runID, NewRenderer(out, c.Opts)), runID), subset: keep, begin: <-begun,
		out: map[string]*JobOutcome{}, done: map[string]chan struct{}{}, marks: map[string]model.Reason{}, cancels: map[string]context.CancelFunc{}, attempts: map[string]string{}}
	sess.Dirty, sess.ConfigOutside, sess.Subset = rev.Dirty, c.ConfigOutside, keep != nil
	sess.AdmitTrust = model.TrustClass(admitted)
	sess.LocalOnly, sess.NodeJob = rs.localOnly, rs.nodeJobFor
	rs.render.Event(Event{Type: "planned", Plan: c.Plan})
	start := time.Now()
	c.race(ctx, h, runID)
	c.drive(ctx, rs, st, coord.ID)
	if end := GitRevision(context.WithoutCancel(ctx), c.Opts.Root, c.capture()); end.SHA != rev.SHA || end.Dirty {
		rev.Dirty = true // the tree moved or changed while jobs ran: the result is not bound to the start commit
	}
	post.Notes = append(post.Notes, dispatchNotes(runID)...)
	res := c.conclude(finishIn{Context: ctx, St: st, RunID: runID, Rev: rev, Source: source, Policy: pol, Hooks: h, Jobs: rs.outcomes(),
		Cancelled: rs.userCancel || ctx.Err() != nil, Partial: c.Opts.Job != "" || c.Opts.Pipeline != "", Start: start, Post: post, Recovered: recovered, Skipped: c.Plan.Skipped, Begin: rs.begin})
	rs.render.Done(res.Summary)
	return res, nil
}

// drive runs the jobs while the heartbeat and the cancel watcher are alive.
func (c *Coordinator) drive(ctx context.Context, rs *runState, st *store.Store, coordID string) {
	bg := context.WithoutCancel(ctx)
	live, stop := context.WithCancel(bg)
	runCtx, cancel := context.WithCancel(ctx)
	rs.cancelRun = cancel
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-live.Done():
				return
			case <-t.C:
				_ = st.Heartbeat(live, coordID)
			}
		}
	}()
	go func() {
		defer wg.Done()
		rs.pollCancels(live)
	}()
	rs.execute(runCtx)
	cancel()
	stop()
	wg.Wait()
}
