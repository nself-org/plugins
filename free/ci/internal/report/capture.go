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
	home, err := os.MkdirTemp("", "nself-ci-cap-")
	if err != nil {
		return "", -1, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	var buf strings.Builder
	var ex ciexec.Executor
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: fmt.Sprintf("cap-%d-%d", time.Now().UnixNano(), captureSeq.Add(1)), CoordinatorID: "capture",
		Job: model.Job{Kind: "static"}, Home: home, Timeout: 30 * time.Second, Command: argv, SourceDir: dir,
		Output: func(b []byte) { buf.Write(b) },
	})
	return buf.String(), res.ExitCode, res.Err
}
