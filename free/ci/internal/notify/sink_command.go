package notify

// Purpose: the command sink: how an external tool (a chat bot, a pager) is attached without
// any vendor name in this repository. The event JSON arrives on the command's stdin.
// Inputs: SinkSpec.Argv. Outputs: success on exit 0; E813 on a start failure, a non-zero
// exit or the 10 second timeout.
// Constraints: argv runs through the exec funnel (internal/exec), never a shell; the
// environment is the funnel's allowlist, so no credential of the operator reaches the
// command; the process group is killed at the timeout; the command's output is dropped
// (it could carry anything) and never read into an error.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// CommandTimeout bounds one command sink run.
const CommandTimeout = 10 * time.Second

// killGrace is how long a command that ignores SIGTERM may outlive its timeout.
const killGrace = 500 * time.Millisecond

// CommandRunner runs argv with stdin and reports its exit code. A timeout, a start failure
// or a cancelled context is an error.
type CommandRunner interface {
	Run(ctx context.Context, argv []string, stdin []byte, timeout time.Duration) (exit int, err error)
}

type commandSink struct {
	name    string
	argv    []string
	runner  CommandRunner
	timeout time.Duration
}

func newCommand(spec SinkSpec, d Deps) (Notifier, error) {
	if len(spec.Argv) == 0 || spec.Argv[0] == "" || spec.URL != "" || spec.Secret != "" {
		return nil, coded("sink " + nameOf(spec) + ": a command sink needs an argv (and no url or secret)")
	}
	r := d.Runner
	if r == nil {
		r = funnelRunner{home: d.Home}
	}
	return &commandSink{name: nameOf(spec), argv: append([]string(nil), spec.Argv...), runner: r, timeout: CommandTimeout}, nil
}

func (c *commandSink) Name() string { return c.name }

func (c *commandSink) Notify(ctx context.Context, ev Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return coded("sink " + c.name + ": event not encodable")
	}
	exit, err := c.runner.Run(ctx, c.argv, append(body, '\n'), c.timeout)
	switch {
	case err != nil:
		return coded(fmt.Sprintf("sink %s: command did not finish: %s", c.name, err))
	case exit != 0:
		return coded(fmt.Sprintf("sink %s: command exited %d", c.name, exit))
	}
	return nil
}

var attempts atomic.Uint64

// funnelRunner is the default CommandRunner: one job through internal/exec. Its home is its
// own state directory: every job start sweeps the process markers of coordinators it does
// not know as live, so it must not be a job home.
type funnelRunner struct{ home string }

func ciHome() string {
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".nself", "ci")
}

func (f funnelRunner) Run(ctx context.Context, argv []string, stdin []byte, timeout time.Duration) (int, error) {
	home := f.home
	if home == "" {
		if home = ciHome(); home == "" {
			return -1, fmt.Errorf("no CI home for the notify funnel")
		}
	}
	spec := ciexec.JobSpec{
		AttemptID:     "notify-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(attempts.Add(1), 10),
		CoordinatorID: "notify",
		Command:       argv,
		Timeout:       timeout,
		Home:          filepath.Join(home, "notify"),
		Stdin:         bytes.NewReader(stdin),
		Output:        func([]byte) {},
	}
	res := (&ciexec.Executor{Env: ciexec.AllowlistEnv{}, Grace: killGrace}).Run(ctx, spec)
	switch {
	case res.Attempt.FailureClass == "timeout":
		return -1, fmt.Errorf("timed out after %s and was killed", timeout)
	case res.Attempt.State == "cancelled":
		return -1, context.Canceled
	case res.Err != nil:
		return -1, fmt.Errorf("could not run")
	}
	return res.ExitCode, nil
}
