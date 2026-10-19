package sample

// Purpose: the default Runner. It runs one argv through the one process funnel
// (internal/exec.Executor), so the readiness hook and the macOS probes get the same
// process-group cleanup, timeout and cancellation as a CI job.
// Inputs: argv, timeout. Outputs: exit code and at most MaxOutput bytes of output (stdout and
// stderr arrive on the one stream; Result.Truncated says when the rest was dropped).
// Constraints: no shell; the environment is PATH and LANG only, never the caller's.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// MaxOutput bounds what a command may return.
const MaxOutput = 64 << 10

// Funnel runs commands through internal/exec. Home is the funnel's own state directory;
// empty means <CI home>/readiness. It must not be the job home: every job start sweeps the
// process markers of coordinators it does not know as live, and would kill a probe in
// flight (the services funnel keeps its own home for the same reason).
type Funnel struct{ Home string }

// killGrace is how long a command that ignores SIGTERM may live past its timeout (the
// executor default of 10 s would turn the 5 s hook timeout into 15 s).
const killGrace = 500 * time.Millisecond

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
	home := f.Home
	if home == "" {
		if home = CIHome(); home == "" {
			return Result{}, fmt.Errorf("no CI home for the readiness funnel")
		}
		home = filepath.Join(home, "readiness")
	}
	var mu sync.Mutex
	var out []byte
	var truncated bool
	spec := ciexec.JobSpec{
		AttemptID:     "readiness-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(attempts.Add(1), 10),
		CoordinatorID: "readiness",
		Command:       argv,
		Timeout:       timeout,
		Home:          home,
		Output: func(p []byte) {
			mu.Lock()
			defer mu.Unlock()
			room := max(MaxOutput-len(out), 0)
			out = append(out, p[:min(room, len(p))]...)
			truncated = truncated || len(p) > room
		},
	}
	res := (&ciexec.Executor{Env: minimalEnv{}, Grace: killGrace}).Run(ctx, spec)
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
	return Result{ExitCode: res.ExitCode, Stdout: string(out), Truncated: truncated}, nil
}
