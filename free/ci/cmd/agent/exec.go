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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	// Stdin is the job's standard input and Masks the exact values its output must not carry
	// (the runner credential: see leaseRunner).
	Stdin io.Reader
	Masks []string
	// RunnerRoot asks the container for the exec-capable runner tmpfs (a runner credential job).
	RunnerRoot bool
	// Entrypoint replaces the container image's entrypoint (a runner credential job on an image whose own
	// entrypoint is a launcher: the agent binary, which then runs the credential wrapper).
	Entrypoint string
	Output     func([]byte)
	// Tail returns the masked output tail for the failure excerpt (nil: use the raw tail).
	Tail func() string
	// Skip names why the job must not run ("cancelled", "secrets_missing").
	Skip string
}

var revisionRE = regexp.MustCompile(`^([0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

func runExecCmd(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	specPath := fs.String("spec", "", "job spec JSON file")
	evPath := fs.String("evidence", "", "where to write the evidence document")
	if err := fs.Parse(args); err != nil || *specPath == "" || *evPath == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent exec --spec <file> --evidence <path>")
		return 2
	}
	if err := RunPreStart(ctx, "exec"); err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	raw, err := os.ReadFile(*specPath)
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 2
	}
	var spec execSpec
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error: spec:", err)
		return 2
	}
	res, err := runSpec(ctx, spec, jobInputs{Output: func(b []byte) { _, _ = sio.Stdout.Write(b) }})
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	if err := writeEvidence(*evPath, res.Evidence); err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
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

// runSpec runs the job once (or, for in.Skip, records why it did not run) and
// returns its outcome and evidence document.
func runSpec(ctx context.Context, spec execSpec, in jobInputs) (session.JobResult, error) {
	if !revisionRE.MatchString(spec.Revision) {
		return session.JobResult{}, errors.New("revision must be a full 40 or 64 hex commit id")
	}
	if spec.JobID == "" {
		spec.JobID = "job"
	}
	rev := cirun.Revision{SHA: spec.Revision}
	builder := evidence.Builder{}
	var begin any
	if spec.SourceDir != "" { // never let git read the agent's own cwd
		begin = builder.Begin(ctx, spec.SourceDir, rev, report.Capture)
	}
	started := time.Now()
	res, tail, err := execute(ctx, spec, in)
	if err != nil {
		return session.JobResult{}, err
	}
	outcome := jobOutcome(spec, res, tail)
	base := baseEvidence(spec, outcome, started, res.Finished)
	done := base
	if spec.SourceDir != "" {
		facts := cirun.EvidenceFacts{Root: spec.SourceDir, RunID: base.RunID, Rev: rev, Git: report.Capture, Jobs: []cirun.JobOutcome{outcome}, Job: spec.JobID}
		var cerr error
		if done, cerr = builder.Complete(ctx, begin, facts, base); cerr != nil {
			done = base
			unbind(&done, "dirty", "evidence assembly failed")
		} else {
			unbindIfOtherRevision(ctx, spec, &done)
		}
	} else {
		unbind(&done, "dirty", "the job ran without a source tree")
	}
	doc, err := json.Marshal(evidence.AttemptDocument(done, outcome))
	return session.JobResult{Outcome: session.Outcome{State: string(res.Attempt.State), ExitCode: res.ExitCode, FailureClass: string(res.Attempt.FailureClass)}, Evidence: doc}, err
}

// execute runs the job through the executor in a per-run work area and returns
// its result and the masked output tail for the failure excerpt.
func execute(ctx context.Context, spec execSpec, in jobInputs) (ciexec.AttemptResult, string, error) {
	now := time.Now()
	switch in.Skip {
	case "cancelled":
		return ciexec.AttemptResult{Attempt: model.Attempt{ID: spec.AttemptID, State: "cancelled", FailureClass: "cancelled"}, ExitCode: -1, Started: now, Finished: now}, "", nil
	case "secrets_missing":
		return ciexec.AttemptResult{Attempt: model.Attempt{ID: spec.AttemptID, State: "failed", FailureClass: "infra"}, ExitCode: -1, Started: now, Finished: now,
			Err: &ciexec.Error{Code: "E612", Message: "declared secrets were not delivered before the lease expired"}}, "", nil
	}
	base, err := workBase()
	if err != nil {
		return ciexec.AttemptResult{}, "", err
	}
	home, err := execHome(base)
	if err != nil {
		return ciexec.AttemptResult{}, "", err
	}
	job := spec.Job
	job.Env = map[string]string{}
	for k, v := range spec.Job.Env {
		job.Env[k] = v
	}
	for k, v := range in.Env {
		job.Env[k] = v
	}
	// The same Isolator chain the local run path uses (hardened container, gVisor, network scope planner);
	// an executor without one would run a container job in the legacy container and a process job on the host.
	iso := cirun.Hooks{}.Resolved().Isolation
	if iso == nil {
		return ciexec.AttemptResult{}, "", errors.New("no isolator is registered; the agent will not run a job without one")
	}
	// The transport expects a cancelled result within five seconds. Leave time
	// for evidence and cleanup after escalating SIGTERM to SIGKILL.
	ex := &ciexec.Executor{Env: ciexec.AllowlistEnv{}, NewRedactor: ciexec.NewOutputRedactor, Stale: agentStale{}, Isolation: iso, Grace: time.Second}
	if len(in.Secrets) > 0 {
		ex.Secrets = mapSecrets(in.Secrets)
	}
	raw := &tailBuffer{max: excerptMax}
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: spec.AttemptID, CoordinatorID: agentIncarnation(spec.CoordinatorID), Job: job, Command: job.Run,
		SourceDir: spec.SourceDir, Mounts: in.Mounts, Home: home, Slot: in.Slot, Stdin: in.Stdin, Masks: in.Masks, RunnerRoot: in.RunnerRoot, Entrypoint: in.Entrypoint,
		CPU: jobCPU(in), MemoryMB: jobMemoryMB(in), BeforeJob: cacheBeforeJob(spec, job.Env), AfterJob: agentAfterJob(spec, in),
		Output: func(b []byte) {
			raw.Write(b)
			if in.Output != nil {
				in.Output(b)
			}
		}})
	if in.Tail != nil { // the session's tail has the lease's secret values masked
		return res, clipTail(in.Tail()), nil
	}
	return res, clipTail(raw.String()), nil
}

// A runner job holds the .NET runner (about 300 to 500 MB) and its memory-only root (the 88 MB bin/ copy in the
// runner tmpfs, charged to the container's memory), so it asks for more than the 1 GB default.
const (
	runnerJobCPU      = 2
	runnerJobMemoryMB = 2048
)

func jobCPU(in jobInputs) float64 {
	if in.RunnerRoot {
		return runnerJobCPU
	}
	return 0
}

func jobMemoryMB(in jobInputs) int {
	if in.RunnerRoot {
		return runnerJobMemoryMB
	}
	return 0
}
