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
// Constraints: a job with no artifacts (and no implicit ones) costs one
// store read; expired records are pruned once per process at the first
// attempt, inside records.AutoPruneBudget; nothing here changes the evidence.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

type artifactsHook struct {
	scanner func() authz.ValueScanner
	env     func(string) string
	now     func() time.Time
	warn    func(string)
	pruned  sync.Map // home -> struct{}: the auto prune ran in this process
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

// AttemptStart records the declarations of the attempt.
func (h *artifactsHook) AttemptStart(ctx context.Context, a AttemptInfo) error {
	if a.Store == nil {
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
	_, epoch, err := a.Store.AttemptStateEpoch(ctx, a.AttemptID)
	if err != nil {
		return err
	}
	if err = a.Store.ArtifactDeclare(ctx, a.AttemptID, epoch, decls); err != nil {
		return err
	}
	if _, seen := h.pruned.LoadOrStore(records.StoreDir(a.Home), struct{}{}); !seen {
		if _, blobs, e := h.setup(a); e == nil {
			_ = records.AutoPrune(ctx, a.Store, blobs, h.now())
		}
	}
	return nil
}

// AfterJob collects the declared outputs from the workspace.
func (h *artifactsHook) AfterJob(ctx context.Context, a AttemptInfo, workspace string) error {
	if a.Store == nil {
		return nil
	}
	decls, err := a.Store.ArtifactDecls(ctx, a.AttemptID)
	if err != nil || len(decls) == 0 {
		return err
	}
	base, err := workdirOf(workspace, a.Job.Workdir)
	if err != nil {
		return err
	}
	pol, blobs, err := h.setup(a)
	if err != nil {
		return err
	}
	_, epoch, err := a.Store.AttemptStateEpoch(ctx, a.AttemptID)
	if err != nil {
		return err
	}
	facts, err := a.Store.ArtifactFacts(ctx, a.AttemptID)
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
	staged, err := records.Collector{Blobs: blobs, Scanner: h.scanner(), Policy: pol}.Collect(ctx, facts.Project, base, decls)
	if err != nil {
		return err
	}
	return a.Store.ArtifactStage(ctx, a.AttemptID, epoch, records.ToRecords(a.AttemptID, facts, epoch, staged, h.now()))
}

// AttemptFinalize verifies and commits the staged records.
func (h *artifactsHook) AttemptFinalize(ctx context.Context, a AttemptInfo, epoch int64) error {
	if a.Store == nil {
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

// workdirOf resolves the job workdir inside the workspace the way the
// executor does: relative, inside, and not through a symlink.
func workdirOf(workspace, workdir string) (string, error) {
	if workdir == "" {
		return workspace, nil
	}
	base := filepath.Join(workspace, workdir)
	if rel, err := filepath.Rel(workspace, base); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", records.Failf(records.ReasonUnreadable, "job workdir %q escapes the workspace", workdir)
	}
	if resolved, err := filepath.EvalSymlinks(base); err != nil || resolved != base {
		return "", records.Failf(records.ReasonUnreadable, "job workdir %q is missing or a symlink", workdir)
	}
	return base, nil
}
