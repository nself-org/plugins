package sample

// Purpose: the default Runner. It runs one argv through the one process funnel
// (internal/exec.Executor), so the readiness hook and the macOS probes get the same
// process-group cleanup, timeout and cancellation as a CI job.
// Inputs: argv, timeout. Outputs: exit code and at most MaxOutput bytes of stdout.
// Constraints: no shell; the environment is PATH and LANG only, never the caller's.

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// MaxOutput bounds what a command may return.
const MaxOutput = 64 << 10

// Funnel runs commands through internal/exec. Home is the CI home (workspace parent);
// empty means NSELF_CI_HOME or ~/.nself/ci, as for a job.
type Funnel struct{ Home string }

var attempts atomic.Uint64

// minimalEnv is the whole environment a funnel command sees.
type minimalEnv struct{}

func (minimalEnv) Environment(context.Context, model.Job) ([]string, error) {
	return []string{"PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C"}, nil
}

// Run implements Runner.
func (f Funnel) Run(ctx context.Context, argv []string, timeout time.Duration) (Result, error) {
	if len(argv) == 0 || argv[0] == "" {
		return Result{}, fmt.Errorf("empty command")
	}
	var mu sync.Mutex
	var out []byte
	spec := ciexec.JobSpec{
		AttemptID:     "readiness-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(attempts.Add(1), 10),
		CoordinatorID: "readiness",
		Command:       argv,
		Timeout:       timeout,
		Home:          f.Home,
		Output: func(p []byte) {
			mu.Lock()
			defer mu.Unlock()
			if room := MaxOutput - len(out); room > 0 {
				out = append(out, p[:min(room, len(p))]...)
			}
		},
	}
	res := (&ciexec.Executor{Env: minimalEnv{}}).Run(ctx, spec)
	mu.Lock()
	defer mu.Unlock()
	switch {
	case res.Attempt.FailureClass == "timeout":
		return Result{}, fmt.Errorf("timed out after %s", timeout)
	case res.Attempt.State == "cancelled":
		return Result{}, context.Canceled
	case res.Err != nil:
		return Result{}, res.Err
	}
	return Result{ExitCode: res.ExitCode, Stdout: string(out)}, nil
}
