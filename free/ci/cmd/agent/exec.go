package main

// Purpose: `nself-ci-agent exec --spec <file> --evidence <path>` runs one job
// through the P7-CI-32 executor and assembles its ci.evidence/v1 document with
// the P7-CI-38 builder. It is the entrypoint hosted workflows (P7-HOST) call,
// and runSpec is what `serve` runs for each lease.
// Inputs:  a JSON job spec file (execSpec); the agent state dir for the
// executor's private work area.
// Outputs: the job's output on stdout, the evidence file (0600, atomic).
// Constraints: exit 0 when the job passed, 1 when it failed or could not run,
// 2 on bad usage; the job runs only through internal/exec; the evidence is
// checked against schemas/ci-evidence.v1.schema.json before it is written.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func init() {
	must(RegisterSubcommand("exec", "run one job spec and write its evidence", runExecCmd))
}

// execSpec is the job a hosted workflow or a test hands to `exec`.
type execSpec struct {
	AttemptID     string    `json:"attempt_id"`
	CoordinatorID string    `json:"coordinator_id"`
	JobID         string    `json:"job_id"`
	Job           model.Job `json:"job"`
	// Revision is the full commit the job runs against (40 or 64 hex).
	Revision  string `json:"revision"`
	SourceDir string `json:"source_dir,omitempty"`
}

// jobInputs are what a lease adds to its spec.
type jobInputs struct {
	Slot    int
	Env     map[string]string
	Secrets map[string]string
	Mounts  []ciexec.Mount
	Output  func([]byte)
	// Mask hides literal secret values from the failure excerpt (nil: none).
	Mask func([]byte) []byte
}

var revisionRE = regexp.MustCompile(`^([0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

func runExecCmd(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	specPath := fs.String("spec", "", "job spec JSON file")
	evPath := fs.String("evidence", "", "where to write the evidence document")
	if err := fs.Parse(args); err != nil || *specPath == "" || *evPath == "" || fs.NArg() != 0 {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent exec --spec <file> --evidence <path>")
		return 2
	}
	raw, err := os.ReadFile(*specPath)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 2
	}
	var spec execSpec
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		fmt.Fprintln(sio.Stderr, "error: spec:", err)
		return 2
	}
	res, err := runSpec(ctx, spec, jobInputs{Output: func(b []byte) { _, _ = sio.Stdout.Write(b) }})
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	if err := writeEvidence(*evPath, res.Evidence); err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	if res.Outcome.State != "passed" {
		return 1
	}
	return 0
}

// writeEvidence validates the document against the published schema and
// writes it atomically with mode 0600.
func writeEvidence(path string, doc []byte) error {
	schemas, err := model.Schemas()
	if err != nil {
		return err
	}
	if err := model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], doc); err != nil {
		return fmt.Errorf("evidence does not validate against ci.evidence/v1: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".evidence-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(doc); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// runSpec runs the job once and returns its outcome and evidence document.
func runSpec(ctx context.Context, spec execSpec, in jobInputs) (session.JobResult, error) {
	if !revisionRE.MatchString(spec.Revision) {
		return session.JobResult{}, errors.New("revision must be a full 40 or 64 hex commit id")
	}
	if spec.JobID == "" {
		spec.JobID = "job"
	}
	job := spec.Job
	job.Env = map[string]string{}
	for k, v := range spec.Job.Env {
		job.Env[k] = v
	}
	for k, v := range in.Env {
		job.Env[k] = v
	}
	home, err := os.MkdirTemp(workBase(), "run-")
	if err != nil {
		return session.JobResult{}, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	ex := &ciexec.Executor{Env: ciexec.AllowlistEnv{}, NewRedactor: ciexec.NewOutputRedactor}
	if len(in.Secrets) > 0 {
		ex.Secrets = mapSecrets(in.Secrets)
	}
	rev := cirun.Revision{SHA: spec.Revision}
	builder := evidence.Builder{}
	begin := builder.Begin(ctx, spec.SourceDir, rev, report.Capture)
	tail := &tailBuffer{max: tailKeep}
	started := time.Now()
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: spec.AttemptID, CoordinatorID: spec.CoordinatorID, Job: job, Command: job.Run,
		SourceDir: spec.SourceDir, Mounts: in.Mounts, Home: home, Slot: in.Slot,
		Output: func(b []byte) {
			tail.Write(b)
			if in.Output != nil {
				in.Output(b)
			}
		}})
	excerpt := tail.String()
	if in.Mask != nil { // mask the whole kept tail first, then clip it
		excerpt = string(in.Mask([]byte(excerpt)))
	}
	excerpt = clipTail(excerpt)
	outcome := jobOutcome(spec, res, excerpt)
	base := baseEvidence(spec, outcome, started, res.Finished)
	facts := cirun.EvidenceFacts{Root: spec.SourceDir, RunID: base.RunID, Rev: rev, Git: report.Capture, Jobs: []cirun.JobOutcome{outcome}, Job: spec.JobID}
	done, err := builder.Complete(ctx, begin, facts, base)
	if err != nil { // unbound and unknown: it can never read as current
		done = base
		why := "evidence assembly failed"
		reason := model.BindingReason("dirty")
		done.Binding, done.WorktreeState = model.Binding{State: "unbound", Reason: &reason}, "dirty"
		done.Freshness = model.Freshness{State: "unknown", Reason: &why}
	}
	if err == nil {
		unbindIfOtherRevision(ctx, spec, &done)
	}
	doc, err := json.Marshal(evidence.AttemptDocument(done, outcome))
	return session.JobResult{Outcome: session.Outcome{State: string(res.Attempt.State), ExitCode: res.ExitCode, FailureClass: string(res.Attempt.FailureClass)}, Evidence: doc}, err
}

// jobOutcome is the run package's record of the job, built from the executor's result.
func jobOutcome(spec execSpec, res ciexec.AttemptResult, tail string) cirun.JobOutcome {
	o := cirun.JobOutcome{ID: spec.JobID, Name: spec.JobID, Check: spec.JobID, Tool: "nself-ci-agent", Kind: spec.Job.Kind,
		Required: true, Substantive: true, FailureClass: res.Attempt.FailureClass, ExitCode: res.ExitCode, AttemptID: spec.AttemptID, N: 1,
		Duration: res.Finished.Sub(res.Started), Tail: tail, Reason: model.Reason(res.Reason)}
	switch res.Attempt.State {
	case "passed":
		o.State = cirun.StatePassed
	case "cancelled":
		o.State = cirun.StateCancelled
	default:
		o.State = cirun.StateFailed
	}
	var ce *ciexec.Error
	if errors.As(res.Err, &ce) {
		o.Code = ce.Code
	}
	return o
}

// baseEvidence is the run document before the P7-CI-38 builder completes it:
// one check for the one job. It mirrors run's coordinator-recorded document
// for a single-job run (run.buildEvidence is unexported; exporting it is
// recorded as debt), with the producer marked self-reported until the result
// signer (P7-TRUST-20) signs it.
func baseEvidence(spec execSpec, o cirun.JobOutcome, start, end time.Time) model.Evidence {
	id := sha256.Sum256([]byte(spec.CoordinatorID + "/" + spec.AttemptID))
	runID := hex.EncodeToString(id[:8])
	e := model.Evidence{
		Schema: "ci.evidence/v1", Revision: spec.Revision, RepoID: "unknown", RunID: runID,
		Pipeline: model.PipelineRef{ID: runID, Name: spec.JobID, Kind: "adhoc"}, Trigger: "api", SourceTrust: "untrusted", // the coordinator classifies the source; the agent claims nothing

		InputDigests: model.InputDigests{Lockfiles: []model.DigestRef{}, Generated: []model.DigestRef{}},
		Runner:       model.RunnerRef{ID: "agent", Provider: "agent", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"},
		Producer:     model.Producer{Kind: "self-reported", Version: "1"}, ToolVersions: map[string]string{}, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required"}, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
		WorktreeState: "clean", Binding: model.Binding{State: "bound"},
	}
	c := model.Check{ID: o.Check, JobID: o.ID, Kind: string(o.Kind), Tool: o.Tool, Required: true, Substantive: true, FailureClass: o.FailureClass, Reason: o.Reason}
	switch o.State {
	case cirun.StatePassed:
		c.Result = "pass"
		e.Counts.Pass = 1
	case cirun.StateFailed:
		c.Result, c.Excerpt = "fail", o.Tail
		e.Counts.Fail = 1
		if o.Code != "" || o.FailureClass == "infra" {
			c.Result = "error"
			e.Counts.Fail, e.Counts.Error = 0, 1
		}
	default:
		c.Result = "skip"
		e.Counts.Skip = 1
	}
	e.Checks, e.Counts.Total = []model.Check{c}, 1
	e.Result = model.Verdict(e.Checks)
	if o.State == cirun.StateCancelled {
		e.Result = "cancelled"
	}
	if c.Result == "error" && e.Result == "pass" {
		e.Result = "error"
	}
	s, f, ms := start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), end.Sub(start).Milliseconds()
	e.Timings = model.Timings{QueuedAt: &s, StartedAt: &s, FinishedAt: &f, DurationMS: &ms}
	return e
}

// unbindIfOtherRevision unbinds evidence whose named revision is not the commit
// the source tree is at: bound, current evidence must describe what ran.
func unbindIfOtherRevision(ctx context.Context, spec execSpec, ev *model.Evidence) {
	if spec.SourceDir == "" {
		return
	}
	out, exit, err := report.Capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", spec.SourceDir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	head := strings.ToLower(strings.TrimSpace(out))
	if err == nil && exit == 0 && head == strings.ToLower(spec.Revision) {
		return
	}
	reason, why := model.BindingReason("sha_mismatch"), "the source tree is not at the named revision"
	ev.Binding = model.Binding{State: "unbound", Reason: &reason}
	ev.Freshness = model.Freshness{State: "unknown", Reason: &why}
}

// workBase is where per-run executor work areas go: outside the agent state dir
// (which holds the keys, and which a job UID may not enter), one 0700 dir per user.
func workBase() string {
	if v := os.Getenv("NSELF_CI_AGENT_WORK"); v != "" {
		_ = os.MkdirAll(v, 0o700)
		return v
	}
	d := filepath.Join(os.TempDir(), fmt.Sprintf("nself-ci-agent-%d", os.Getuid()))
	_ = os.MkdirAll(d, 0o700)
	return d
}

func mapSecrets(m map[string]string) ciexec.SecretSource { return secretMap(m) }

type secretMap map[string]string

func (s secretMap) Resolve(_ context.Context, refs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range refs {
		v, ok := s[r]
		if !ok {
			return nil, fmt.Errorf("E612: secret %q was not delivered", r)
		}
		out[r] = v
	}
	return out, nil
}

// tailBuffer keeps the last max bytes of output for the failure excerpt.
type tailBuffer struct {
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
}
func (t *tailBuffer) String() string { return string(t.buf) }

const (
	// tailKeep is the raw output kept for the failure excerpt: enough that a
	// secret value up to the frame cap is whole when it is masked.
	tailKeep = 80 << 10
	// excerptMax is the excerpt that goes into the evidence.
	excerptMax = 2000
)

func clipTail(s string) string {
	if len(s) <= excerptMax {
		return s
	}
	s = s[len(s)-excerptMax:]
	return strings.ToValidUTF8(s, "")
}
