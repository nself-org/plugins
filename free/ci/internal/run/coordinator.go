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
	"github.com/nself-org/plugins/free/ci/internal/policy"
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
	if got != "owner" && got != "internal" {
		// ADR 0025 §2-3, §6: collaborator and untrusted work needs an isolated runner, and this
		// coordinator runs jobs as the operator. The one admitted exception is a pull request on a
		// disposable hosted runner (Option A): it runs with no secrets, no status write and a
		// restricted network. A persistent or unknown runner refuses it.
		if !pullRequestRun(c.Plan) || !disposableHostedRunner(os.Getenv) {
			return "", "", fail(CodeTrust, "the source is classed %s: it needs an isolated runner, and this coordinator runs jobs as the operator", got)
		}
		c.Opts.NoStatus = true
		if c.Opts.Network != "none" {
			c.Opts.Network = "restricted"
		}
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

// pullRequestRun reports whether the plan was selected for a pull_request trigger.
func pullRequestRun(p *plan.Plan) bool { return p != nil && p.Trigger == "pull_request" }

// disposableHostedRunner is true only on a GitHub-hosted runner (one job, destroyed after). A
// self-hosted runner reports RUNNER_ENVIRONMENT=self-hosted and is treated as persistent.
func disposableHostedRunner(getenv func(string) string) bool {
	return getenv("GITHUB_ACTIONS") == "true" && getenv("RUNNER_ENVIRONMENT") == "github-hosted"
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
