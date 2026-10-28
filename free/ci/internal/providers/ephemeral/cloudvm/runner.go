package cloudvm

// Purpose: the exec funnel for one `nself infra` child (CI D26): the child runs through
// internal/exec with a fixed argv and an environment built for that one call.
// Inputs:  an absolute argv, the exact environment, the secret values the output must never carry.
// Outputs: the combined output (capped) and the exit code.
// Constraints: the environment is a closed list given by the caller (the funnel adds only its own
// NSELF_CI_EXEC_ATTEMPT marker); nothing is inherited from the coordinator; the job has no
// timeout field (a ulimit wrapper would add shell variables), the caller's context bounds it;
// every secret value is masked in the output, even split across reads; stdin is empty.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// maxOutput caps what one child may hand back (an envelope is a few KiB).
const maxOutput = 1 << 20

// killGrace is how long a child that ignores SIGTERM may outlive its context.
const killGrace = 2 * time.Second

// Runner runs one child and returns its combined output.
type Runner interface {
	Run(ctx context.Context, argv, env []string, mask []string) (out []byte, exitCode int, err error)
}

// fixedEnv is the EnvPolicy that returns exactly the given entries.
type fixedEnv []string

func (f fixedEnv) Environment(context.Context, model.Job) ([]string, error) {
	return append([]string(nil), f...), nil
}

var attempts atomic.Uint64

// funnelRunner is the default Runner: one job through internal/exec. Its home is its own state
// directory, not a job home.
type funnelRunner struct {
	home, coordinator string
}

func (f funnelRunner) Run(ctx context.Context, argv, env, mask []string) ([]byte, int, error) {
	var mu sync.Mutex
	var buf bytes.Buffer
	spec := ciexec.JobSpec{
		AttemptID:     "cloudvm-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(attempts.Add(1), 10),
		CoordinatorID: f.coordinator,
		Command:       argv,
		Home:          filepath.Join(f.home, "cloudvm"),
		Masks:         mask,
		Output: func(b []byte) {
			mu.Lock()
			defer mu.Unlock()
			if room := maxOutput - buf.Len(); room > 0 {
				buf.Write(b[:min(len(b), room)])
			}
		},
	}
	res := (&ciexec.Executor{Isolation: ciexec.BaselineIsolator{}, Env: fixedEnv(env), Grace: killGrace}).Run(ctx, spec)
	mu.Lock()
	defer mu.Unlock()
	if res.Err != nil {
		return buf.Bytes(), res.ExitCode, res.Err
	}
	return buf.Bytes(), res.ExitCode, nil
}
