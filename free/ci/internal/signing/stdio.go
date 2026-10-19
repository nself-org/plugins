package signing

// Purpose: run a child process through the exec funnel with bytes on its
// stdin (Epic CACHE D12): the signer, and `nself config secrets set`, which
// reads the key from stdin so it never sits on argv.
// Inputs:  a StdioCall (argv, extra environment, working directory, stdin).
// Outputs: the child's combined output (capped) and exit code.
// Constraints: the bytes travel on the funnel's JobSpec.Stdin, so nothing
// reaches disk; a working directory other than the funnel's workspace needs
// a small `sh -c cd` wrapper (unix only, see chdir_unix.go); the environment
// is the funnel allowlist plus the call's additions; the output buffer is
// returned to the caller, who zeroes it when it can carry a secret.

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// maxOutput bounds what a call may print.
const maxOutput = 5 << 20

// StdioCall is one child process.
type StdioCall struct {
	Argv []string
	// Env is added to the funnel's allowlisted environment.
	Env []string
	// Dir is the child's working directory ("" keeps the funnel's workspace).
	Dir string
	// Stdin is the child's standard input (nil: /dev/null).
	Stdin   []byte
	Timeout time.Duration
}

// notRun is the exit code reported with an error: the program did not run (-1).
const notRun = ^0

// StdioRunner runs one call. A non-nil error means the program could not be
// run at all; a non-zero exit code is not an error.
type StdioRunner interface {
	RunStdio(ctx context.Context, c StdioCall) ([]byte, int, error)
}

// extraEnv adds fixed entries to the funnel's allowlisted environment.
type extraEnv []string

func (e extraEnv) Environment(ctx context.Context, j model.Job) ([]string, error) {
	base, err := ciexec.AllowlistEnv{}.Environment(ctx, j)
	return append(base, e...), err
}

// passRedactor keeps output unmasked: a response is data, not a log.
type passRedactor struct{}

func (passRedactor) Redact(p []byte) []byte { return p }
func (passRedactor) Flush() []byte          { return nil }

// FunnelRunner is the production StdioRunner.
type FunnelRunner struct {
	// Home is the CI state directory the funnel works under ("" = its default).
	Home string
	seq  atomic.Uint64
}

type capture struct {
	mu  sync.Mutex
	buf []byte
}

// write keeps at most maxOutput bytes in total.
func (c *capture) write(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, b[:min(len(b), max(maxOutput-len(c.buf), 0))]...)
}

// RunStdio implements StdioRunner.
func (r *FunnelRunner) RunStdio(ctx context.Context, c StdioCall) ([]byte, int, error) {
	if len(c.Argv) == 0 {
		return nil, notRun, Failf(ReasonSignerFailed, "no program to run")
	}
	argv := c.Argv
	if c.Dir != "" && c.Dir != "." {
		var err error
		if argv, err = inDir(c.Dir, argv); err != nil {
			return nil, notRun, err
		}
	}
	var stdin io.Reader
	if c.Stdin != nil {
		stdin = bytes.NewReader(c.Stdin)
	}
	var cap capture
	ex := &ciexec.Executor{Env: extraEnv(c.Env), NewRedactor: func() ciexec.Redactor { return passRedactor{} }}
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: r.id(), CoordinatorID: "signer", Command: argv, Home: r.Home, Timeout: c.Timeout, Output: cap.write, Stdin: stdin})
	if res.Err != nil {
		clear(cap.buf)
		return nil, notRun, Failf(ReasonSignerFailed, "the program could not be run or timed out")
	}
	return cap.buf, res.ExitCode, nil
}

func (r *FunnelRunner) id() string {
	return "sig-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(r.seq.Add(1), 10)
}
