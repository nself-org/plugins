package secrets

// Purpose: the production Runner: every cli call goes through the exec funnel (the only package that
// may import os/exec, Epic D15), the same way the services package runs the nself and docker CLIs.
// Inputs:  a Call (argv, extra environment, timeout).
// Outputs: the call's combined output, capped, and its exit code.
// Constraints: the environment is the exec allowlist plus the call's additions (never the whole
// operator environment); output is captured to a byte buffer by a pass-through redactor, because the
// resolver's answer IS the secret and must reach the parser unmasked; it is never logged or shown.

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ExecRunner runs calls through ciexec.Executor.
type ExecRunner struct {
	// Home is the CI state directory the funnel works under ("" = the exec default).
	Home string
	// CoordinatorID labels the funnel's workspaces; "" means "secrets".
	CoordinatorID string
	// Attempt is the attempt the calls serve; it prefixes the funnel's own call ids.
	Attempt string
	// Env is added to the environment of every call (a call's own Env follows it).
	Env []string

	seq atomic.Uint64
}

type callEnv []string

func (e callEnv) Environment(ctx context.Context, j model.Job) ([]string, error) {
	base, err := ciexec.AllowlistEnv{}.Environment(ctx, j)
	return append(base, e...), err
}

// capture collects output; Redact returns nil so nothing flows on, Flush is empty.
type capture struct {
	mu  sync.Mutex
	buf []byte
}

func (c *capture) write(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.buf) <= maxOutput {
		c.buf = append(c.buf, b...)
	}
}

type passRedactor struct{}

func (passRedactor) Redact(p []byte) []byte { return p }
func (passRedactor) Flush() []byte          { return nil }

func (x *ExecRunner) id() string {
	a := x.Attempt
	if a == "" || len(a) > 100 {
		a = "call"
	}
	return "sec-" + sanitize(a) + "-" + strconv.FormatUint(x.seq.Add(1), 10)
}

// sanitize keeps the exec funnel's id alphabet.
func sanitize(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			b[i] = '_'
		}
	}
	return string(b)
}

// Run implements Runner.
func (x *ExecRunner) Run(ctx context.Context, c Call) (Output, error) {
	coord := x.CoordinatorID
	if coord == "" {
		coord = "secrets"
	}
	ex := &ciexec.Executor{Env: callEnv(append(append([]string(nil), x.Env...), c.Env...)), NewRedactor: func() ciexec.Redactor { return passRedactor{} }}
	var cap capture
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: x.id(), CoordinatorID: coord, Command: c.Argv, Home: x.Home, Timeout: c.Timeout, Output: cap.write})
	out := Output{Bytes: cap.buf, ExitCode: res.ExitCode}
	if res.Err != nil {
		zero(cap.buf)
		return Output{}, res.Err
	}
	return out, nil
}
