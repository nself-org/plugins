package run

// Purpose: the PERSONAL coordinator (ADR 0024 §4): the invoking process
// registers an identity, recovers after dead coordinators, persists the
// planned run and executes it through CreateAttempt and the executor, then
// records evidence and posts the one status (finish.go).
// Inputs:  a Coordinator holding Options, the built plan and the hooks.
// Outputs: a Result with the exit code and the Summary the renderer showed.
// Constraints: the plan is built by the caller (detect and config live above
// this package); a statically unplaceable required job fails the run before
// anything is persisted (E618); every cleanup write survives Ctrl-C.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Coordinator runs one planned pipeline on this host.
type Coordinator struct {
	Opts         Options
	Hooks        Hooks
	Plan         *plan.Plan
	ConfigDigest string
	// ConfigOutside is true when the config is not the tracked file in Root
	// (--config elsewhere): such a run is never bound to the commit.
	ConfigOutside bool
	// Substantive says whether a job counts as substantive evidence; nil
	// means every job does.
	Substantive func(plan.PlannedJob) bool
	Out         io.Writer
	// Capture runs git and gh; nil uses the exec funnel.
	Capture Capture
	// Reporter posts the status; nil resolves the GitHub remote and gh auth.
	Reporter  report.Reporter
	Capacity  func() ciexec.Capacity
	Admission ciexec.AdmissionPolicy
	Poll      time.Duration

	exec *ciexec.Executor
}

// Result is how a run ended.
type Result struct {
	RunID   string
	Exit    int
	Summary Summary
}

func newRunID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "r" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

// rowID names the job row of a planned job; it must stay a valid executor id.
func rowID(runID, job string) string {
	// the digest keeps two job ids that safeID maps to one string apart
	s := safeID(job)
	if len(s) > 40 {
		s = s[:40]
	}
	return runID + "." + s + "-" + shortDigest(job)[:8]
}

func (c *Coordinator) capture() Capture {
	if c.Capture != nil {
		return c.Capture
	}
	return report.Capture
}

// classify returns the class the run is admitted at and the class it records. They differ only for
// an embedding handler (ad-hoc) that reports a lower RecordedTrust: scheduling keeps the admitted
// class, the pipeline row and evidence keep the recorded one.
func (c *Coordinator) classify(ctx context.Context, h Hooks, rev Revision) (admitted, recorded string, err error) {
	if h.Trust == nil {
		return "", "", fail(CodeTrust, "no source trust classifier registered; source is untrusted")
	}
	got, err := h.Trust.Classify(ctx, TrustRequest{Root: c.Opts.Root, Revision: rev.SHA, Requested: c.Opts.Trust})
	if err != nil {
		return "", "", err
	}
	if got != "owner" && got != "internal" { // ADR 0025 §2-3: collaborator and untrusted work needs an isolated runner; this coordinator runs jobs as the operator
		return "", "", fail(CodeTrust, "the source is classed %s: it needs an isolated runner, and this coordinator runs jobs as the operator", got)
	}
	if c.Opts.Trust != "" && string(got) != c.Opts.Trust {
		return "", "", fail(CodeTrust, "the source is classed %s, not %s", got, c.Opts.Trust)
	}
	if recorder, ok := h.Trust.(RecordedTrustClassifier); ok {
		recorded := recorder.RecordedTrust()
		if !recorded.Valid() || (got == "internal" && recorded == "owner") {
			return "", "", fail(CodeTrust, "invalid recorded source trust %q for admitted class %s", recorded, got)
		}
		return string(got), string(recorded), nil
	}
	return string(got), string(got), nil
}

func (c *Coordinator) policy(ctx context.Context, h Hooks, class model.TrustClass) (PolicyResult, error) {
	var r PolicyResolver = defaultResolver{}
	if h.Policy != nil {
		r = h.Policy
	}
	return r.Resolve(ctx, PolicyRequest{Root: c.Opts.Root, Store: c.storePath(), Pipeline: c.Plan.Pipeline.Name, Run: c.Opts.PolicyRun,
		Class: class, Visibility: policyVisibility(c.Opts.ProviderRepo)})
}

func (c *Coordinator) storePath() string {
	if c.Opts.Home == "" {
		return ""
	}
	return filepath.Join(c.Opts.Home, "state.db")
}

// persist writes the pipeline and its jobs.
func (c *Coordinator) persist(ctx context.Context, st *store.Store, runID string, rev Revision, source string, pol PolicyResult, sess *Session, mp *model.Pipeline) (map[string]string, error) {
	trig := string(c.Plan.Trigger)
	if trig == "" {
		trig = "local"
	}
	row := store.PipelineRow{ID: runID, Project: c.Opts.Root, Revision: rev.SHA, Trigger: trig, SourceTrust: source, PrivacyZone: "local-only",
		PolicyDigest: pol.Digest, InputDigest: c.ConfigDigest, SelectionMode: string(c.Plan.Selection.Mode), SelectionReason: string(c.Plan.Selection.Reason),
		SelectionDigest: c.Plan.Selection.Digest, Status: "running"}
	if err := st.CreatePipeline(ctx, row); err != nil {
		return nil, err
	}
	rows := map[string]string{}
	for i, pj := range c.Plan.Jobs {
		body, _ := json.Marshal(pj.Job)
		sum := sha256.Sum256(body)
		j := pj.Job
		infra := 0
		if j.Retry != nil {
			infra = j.Retry.InfraMax
		}
		jr := store.JobRow{ID: rowID(runID, pj.ID), PipelineID: runID, Name: pj.ID, Kind: string(j.Kind), InputDigest: hex.EncodeToString(sum[:]),
			Idempotent: j.Kind != "release" && (j.Idempotent == nil || *j.Idempotent), Required: j.Required != nil && *j.Required, InfraMax: infra}
		if err := st.CreateJob(ctx, jr); err != nil {
			return nil, err
		}
		rows[pj.ID] = jr.ID
		sess.bind(&mp.Jobs[i], jr.ID)
	}
	return rows, nil
}

// Run executes the plan. A returned error is an infrastructure or usage
// failure (exit 2 or 1 by its code); a failed gate is a Result with Exit 1.
func (c *Coordinator) Run(ctx context.Context) (Result, error) {
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
	ciexec.WarmRuntimeInfo() // the first job's orphan sweep asks the daemon: let the answer arrive while git and policy run
	rev := GitRevision(ctx, c.Opts.Root, c.capture())
	rev.Dirty = rev.Dirty || c.ConfigOutside
	checkout, release, err := c.offload(ctx, h, rev) // a snapshot is what nodes check out; the run records rev
	if err != nil {
		return Result{}, err
	}
	defer release()
	rev.Dirty = rev.Dirty || checkout != rev.SHA // a run whose nodes test a snapshot is a dev run: never a status of record
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
	defer dispatchRelease(runID) // also closes the dispatcher of a run that ends before AfterRun (D-1044)
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
	sess.Placement, sess.Queue = pol.Placement, pol.Queue
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
	rs := &runState{source: model.TrustClass(source), c: c, sess: sess, hooks: h, pol: pol, pl: c.Plan, mp: mp, rev: rev, rows: rows, subst: subst, render: stampRun(journal(ctx, st, runID, NewRenderer(out, c.Opts)), runID), subset: keep, begin: c.beginEvidence(ctx, h, rev),
		out: map[string]*JobOutcome{}, done: map[string]chan struct{}{}, marks: map[string]model.Reason{}, cancels: map[string]context.CancelFunc{}, attempts: map[string]string{}}
	sess.Dirty, sess.ConfigOutside, sess.Subset = rev.Dirty, c.ConfigOutside, keep != nil
	sess.AdmitTrust = model.TrustClass(admitted)
	sess.LocalOnly = rs.localOnly
	rs.render.Event(Event{Type: "planned", Plan: c.Plan})
	start := time.Now()
	c.race(ctx, h, runID)
	c.drive(ctx, rs, st, coord.ID)
	if end := GitRevision(context.WithoutCancel(ctx), c.Opts.Root, c.capture()); end.SHA != rev.SHA || end.Dirty {
		rev.Dirty = true // the tree moved or changed while jobs ran: the result is not bound to the start commit
	}
	res := c.conclude(finishIn{St: st, RunID: runID, Rev: rev, Source: source, Policy: pol, Hooks: h, Jobs: rs.outcomes(),
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
