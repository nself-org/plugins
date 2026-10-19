package run

// Purpose: wire artifact records into the run (P7-CACHE-03, Epic D6). At
// attempt start the declared outputs are recorded; when the job's command
// exits 0 its outputs are collected from the workspace into the project realm
// of the blob store (scan first, path sanitiser on every entry); while the
// attempt is finalizing CommitAndVerify re-hashes the blobs and commits, so
// the attempt only passes with verified artifacts (R-DS-20).
// Inputs:  AttemptInfo (job config, store, home), the process-wide secret
// registry (Redactions), NSELF_CI_* caps from the environment.
// Outputs: records in the store, blobs in $NSELF_CI_HOME/blobs, a refusal
// (E712/E683/E711) that fails the attempt with class infra.
// Constraints: a job with no artifacts (and no implicit ones) never touches the
// store from these hooks; expired records are pruned at a declaring attempt at
// most every 6 hours per store, inside records.AutoPruneBudget; nothing here
// changes the evidence.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type artifactsHook struct {
	scanner func() authz.ValueScanner
	env     func(string) string
	now     func() time.Time
	warn    func(string)
	pruned  sync.Map // store dir -> time.Time of the last automatic prune
}

var artifactsOn = &artifactsHook{scanner: func() authz.ValueScanner { return Redactions }, env: os.Getenv, now: time.Now,
	warn: func(w string) { fmt.Fprintln(os.Stderr, "nself-ci: warning: "+w) }}

func init() {
	RegisterAttemptStart(artifactsOn)
	RegisterWorkspace(artifactsOn)
	RegisterAttemptFinalize(artifactsOn)
}

func (h *artifactsHook) setup(a AttemptInfo) (records.Policy, *cas.Store, error) {
	pol, err := records.PolicyFromEnv(h.env)
	if err != nil {
		return pol, nil, err
	}
	home := records.StoreDir(a.Home)
	pol.StorePath, pol.Warn = home, h.warn
	pol.Team = h.env("NSELF_CI_COORDINATOR_MODE") == "team"
	blobs, err := cas.New(home)
	return pol, blobs, err
}

// declares reports whether the job declares any output, without touching the
// store: a job that does not pays nothing and cannot be failed by this hook.
func declares(job model.Job) bool {
	return len(job.Artifacts) > 0 || len(records.ImplicitDecls(job)) > 0
}

// epochOf is the capability the attempt launched with. A caller that did not
// set it gets a refusal: reading "the current epoch" here would let a stale
// callback adopt a later incarnation's authority.
func epochOf(a AttemptInfo) (int64, error) {
	if a.Epoch == 0 {
		return 0, records.Failf(records.ReasonEpoch, "the attempt carries no launch epoch")
	}
	return a.Epoch, nil
}

// scrub keeps a registered secret out of an error that reaches run output:
// workspace paths appear in messages, and the output redactor does not see them.
func (h *artifactsHook) scrub(err error) error {
	if err == nil {
		return nil
	}
	sc := h.scanner()
	if sc == nil {
		return records.Failf(records.ReasonSecretMatch, "no secret scanner is registered; the error text was withheld")
	}
	if len(sc.Scan([]byte(err.Error()))) == 0 {
		return err
	}
	var re *records.Error
	if errors.As(err, &re) {
		return &records.Error{ID: re.ID, Reason: re.Reason, Msg: "the message named a registered secret value and was withheld"}
	}
	return records.Failf(records.ReasonSecretMatch, "an error named a registered secret value and was withheld")
}

// AttemptStart records the declarations of the attempt.
func (h *artifactsHook) AttemptStart(ctx context.Context, a AttemptInfo) error {
	return h.scrub(h.attemptStart(ctx, a))
}

func (h *artifactsHook) attemptStart(ctx context.Context, a AttemptInfo) error {
	if a.Store == nil || !declares(a.Job) {
		return nil
	}
	facts, err := a.Store.ArtifactFacts(ctx, a.AttemptID)
	if err != nil {
		return err
	}
	decls, err := records.Normalize(a.Job, facts.Release, records.ImplicitDecls(a.Job))
	if err != nil || len(decls) == 0 {
		return err
	}
	for _, d := range decls { // declaration text reaches records and list output
		if sc := h.scanner(); sc == nil || len(sc.Scan([]byte(d.Path+"\n"+d.Name+"\n"+d.ImageRef))) != 0 {
			return records.Failf(records.ReasonSecretMatch, "an artifact declaration holds a registered secret value (or no scanner is registered); nothing is declared")
		}
	}
	epoch, err := epochOf(a)
	if err != nil {
		return err
	}
	if err = a.Store.ArtifactDeclare(ctx, a.AttemptID, epoch, decls); err != nil {
		return err
	}
	h.autoPrune(ctx, a)
	return nil
}

// pruneEvery is how often a long-lived process runs the automatic prune per store.
const pruneEvery = 6 * time.Hour

// autoPrune removes expired records and unreferenced blobs, at most once per
// pruneEvery per store directory; a failure is a warning, never a job failure.
func (h *artifactsHook) autoPrune(ctx context.Context, a AttemptInfo) {
	dir, now := records.StoreDir(a.Home), h.now()
	if last, ok := h.pruned.Load(dir); ok && !now.Before(last.(time.Time)) && now.Sub(last.(time.Time)) < pruneEvery {
		return // a clock that moved backwards prunes again rather than stalling
	}
	h.pruned.Store(dir, now)
	_, blobs, err := h.setup(a)
	if err == nil {
		err = records.AutoPrune(ctx, a.Store, blobs, now)
	}
	if err != nil {
		h.warn("artifact auto prune: " + firstLineOf(err.Error()))
	}
}

// AfterJob collects the declared outputs from the workspace the executor pinned.
func (h *artifactsHook) AfterJob(ctx context.Context, a AttemptInfo, workspace *os.Root) error {
	return h.scrub(h.afterJob(ctx, a, workspace))
}

func (h *artifactsHook) afterJob(ctx context.Context, a AttemptInfo, workspace *os.Root) error {
	if a.Store == nil || !declares(a.Job) {
		return nil
	}
	decls, err := a.Store.ArtifactDecls(ctx, a.AttemptID)
	if err != nil || len(decls) == 0 {
		return err
	}
	root, closeRoot, err := workdirRoot(workspace, a.Job.Workdir)
	if err != nil {
		return err
	}
	defer closeRoot()
	pol, blobs, err := h.setup(a)
	if err != nil {
		return err
	}
	epoch, err := epochOf(a)
	if err != nil {
		return err
	}
	facts, err := a.Store.ArtifactFacts(ctx, a.AttemptID)
	if err != nil {
		return err
	}
	usage, err := a.Store.ArtifactUsage(ctx, facts.Project)
	if err != nil {
		return err
	}
	if facts.Platform == "" {
		facts.Platform = runtime.GOOS + "/" + runtime.GOARCH
	}
	if facts.Isolation = string(a.Job.Isolation); facts.Isolation == "" {
		facts.Isolation = "process"
	}
	facts.Producer = "nself-ci/" + a.JobName
	reserve := func(total int64) error { return a.Store.ArtifactReserve(ctx, a.AttemptID, epoch, total, pol) }
	staged, err := records.Collector{Blobs: blobs, Scanner: h.scanner(), Policy: pol, Usage: usage, Reserve: reserve}.CollectRoot(ctx, facts.Project, root, decls)
	if err != nil {
		return err
	}
	return a.Store.ArtifactStage(ctx, a.AttemptID, epoch, records.ToRecords(a.AttemptID, facts, epoch, staged, h.now()))
}

// Wants lets the run skip the finalize read for jobs that declare nothing.
func (h *artifactsHook) Wants(a AttemptInfo) bool { return declares(a.Job) }

// AttemptFinalize verifies and commits the staged records.
func (h *artifactsHook) AttemptFinalize(ctx context.Context, a AttemptInfo, epoch int64) error {
	if a.Store == nil || !declares(a.Job) {
		return nil
	}
	decls, err := a.Store.ArtifactDecls(ctx, a.AttemptID)
	if err != nil || len(decls) == 0 {
		return err
	}
	pol, blobs, err := h.setup(a)
	if err != nil {
		return err
	}
	_, err = records.CommitAndVerify(ctx, a.Store, blobs, a.AttemptID, epoch, pol)
	return err
}

// workdirRoot returns the job workdir inside the pinned workspace the way the
// executor resolves it: relative, inside, and not a symlink. The returned
// close func is a no-op when the workdir is the workspace itself.
func workdirRoot(workspace *os.Root, workdir string) (*os.Root, func(), error) {
	if workdir == "" {
		return workspace, func() {}, nil
	}
	info, err := workspace.Lstat(workdir)
	if err != nil || !info.IsDir() {
		return nil, nil, records.Failf(records.ReasonUnreadable, "job workdir %q is missing, not a directory or a symlink", workdir)
	}
	sub, err := workspace.OpenRoot(workdir)
	if err != nil {
		return nil, nil, records.Failf(records.ReasonUnreadable, "job workdir %q escapes the workspace: %v", workdir, err)
	}
	return sub, func() { _ = sub.Close() }, nil
}
