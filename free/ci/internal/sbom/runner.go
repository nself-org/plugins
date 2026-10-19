package sbom

// Purpose: the one seam between the SBOM step and process execution.
// Inputs:  a Cmd (argv, env, files to read back).
// Outputs: Result (output, exit code, the collected files).
// Constraints: commands run only through internal/exec; ExecRunner gives each
// command a fresh private workspace copied from SourceDir, so the output files
// are read back (size-capped) before it is removed; a job-integrated Runner
// (the release hook) runs the same Cmd inside the job's own workspace.

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// MaxDocBytes bounds one collected SBOM file.
const MaxDocBytes = 256 << 20

// maxOutput bounds the command output kept for diagnostics.
const maxOutput = 1 << 20

// Result is the outcome of one Cmd.
type Result struct {
	Output string
	Exit   int
	Files  map[string][]byte
}

// Runner executes a Cmd in the job workdir.
type Runner interface {
	Run(ctx context.Context, c Cmd) (Result, error)
}

var runSeq atomic.Uint64

// ExecRunner runs commands through an exec.Executor.
type ExecRunner struct {
	Executor  *exec.Executor
	Home      string                 // NSELF_CI_HOME ("" = the executor's default)
	SourceDir string                 // copied into the workspace before the command
	Isolation string                 // "process" (default) or "container"
	Container *model.ContainerConfig // required when Isolation is "container" (the toolchain image)
	Timeout   time.Duration          // 0 = 10 minutes
}

// IsolationName is the isolation commands run under ("process" or "container").
func (r ExecRunner) IsolationName() string {
	if r.Isolation == "" {
		return "process"
	}
	return r.Isolation
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, c Cmd) (Result, error) {
	if r.Executor == nil {
		return Result{}, fmt.Errorf("executor required")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	iso := r.IsolationName()
	var out strings.Builder
	res := Result{Files: map[string][]byte{}}
	var collectErr error
	spec := exec.JobSpec{
		AttemptID:     fmt.Sprintf("sbom-%d-%d", os.Getpid(), runSeq.Add(1)),
		CoordinatorID: "sbom",
		Job:           model.Job{Kind: "release", Workdir: ".", Isolation: model.Isolation(iso), Container: r.Container, Env: c.Env},
		Command:       c.Argv,
		SourceDir:     r.SourceDir,
		Home:          r.Home,
		Timeout:       timeout,
		Output: func(p []byte) {
			if room := maxOutput - out.Len(); room > 0 {
				out.Write(p[:min(len(p), room)])
			}
		},
		AfterJob: func(_ context.Context, ws *os.Root) error {
			for _, name := range c.Collect {
				b, err := readCapped(ws, name)
				if err != nil {
					collectErr = err
					return nil // the caller names the missing file
				}
				res.Files[name] = b
			}
			return nil
		},
	}
	ar := r.Executor.Run(ctx, spec)
	res.Output, res.Exit = out.String(), ar.ExitCode
	if ar.Err != nil {
		return res, ar.Err
	}
	if collectErr != nil && res.Exit == 0 {
		return res, collectErr
	}
	return res, nil
}

func readCapped(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, MaxDocBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxDocBytes {
		return nil, Failf(ReasonInvalid, "%s is larger than %d bytes", name, MaxDocBytes)
	}
	return b, nil
}
