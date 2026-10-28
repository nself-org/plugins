package report

// Purpose: run one short process through the exec funnel and capture its
// output (git, gh). The internal/exec package is the only place that starts
// processes; this is the thin caller everything else in the run path uses.
// Inputs:  argv. Outputs: stdout and stderr merged, the exit code.
// Constraints: 30 s timeout unless ctx is shorter; no stdin.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type noLazyFetchKey struct{}

// WithoutLazyFetch prevents git reads from fetching missing partial-clone objects.
func WithoutLazyFetch(ctx context.Context) context.Context {
	return context.WithValue(ctx, noLazyFetchKey{}, true)
}

var captureSeq atomic.Int64

// Capture runs argv in an empty private workspace and returns its combined
// output. A non-zero exit is not an error; err is set only when the process
// could not be run.
func Capture(ctx context.Context, argv ...string) (out string, exit int, err error) {
	return CaptureIn(ctx, "", argv...)
}

// CaptureIn is Capture with the workspace seeded from the tree at dir, as the
// gate runner does, so a tool that resolves its version by working directory
// (a go.mod toolchain line, .tool-versions, .nvmrc) answers for that tree. An
// empty dir gives the empty workspace.
func CaptureIn(ctx context.Context, dir string, argv ...string) (out string, exit int, err error) {
	return CaptureAs(ctx, model.Job{Kind: "static"}, dir, argv...)
}

// CaptureAs is CaptureIn for a job: the process runs in the job's workdir under
// the workspace seeded from dir, with the job's own env added to the default
// allowlist, as the job itself would run (a corepack pin, a rust-toolchain.toml
// or a go.mod toolchain line then answers for that directory).
func CaptureAs(ctx context.Context, job model.Job, dir string, argv ...string) (out string, exit int, err error) {
	home, err := os.MkdirTemp("", "nself-ci-cap-")
	if err != nil {
		return "", -1, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	var buf strings.Builder
	var ex ciexec.Executor
	if ctx.Value(noLazyFetchKey{}) == true {
		env := make(map[string]string, len(job.Env)+1)
		for k, v := range job.Env {
			env[k] = v
		}
		env["GIT_NO_LAZY_FETCH"] = "1"
		job.Env = env
	}
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: fmt.Sprintf("cap-%d-%d", time.Now().UnixNano(), captureSeq.Add(1)), CoordinatorID: "capture",
		Job: job, Home: home, Timeout: 30 * time.Second, Command: argv, SourceDir: dir,
		Output: func(b []byte) { buf.Write(b) },
	})
	return buf.String(), res.ExitCode, res.Err
}
