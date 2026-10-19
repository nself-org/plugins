package signing

// Purpose: run a child process through the exec funnel with bytes on its
// stdin (Epic CACHE D12): the signer, and `nself config secrets set`, which
// reads the key from stdin so it never sits on argv.
// Inputs:  a StdioCall (argv, extra environment, working directory, stdin).
// Outputs: the child's combined output (capped) and exit code.
// Constraints: the funnel (internal/exec) has no stdin field, so the bytes go
// through a FIFO in a private 0700 directory that a `sh -c` wrapper redirects
// from; a FIFO holds no data once both ends close, so a key never reaches
// disk; the environment is the funnel allowlist plus the call's additions;
// the output buffer is returned to the caller, who zeroes it when it can
// carry a secret.

import (
	"context"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// maxOutput bounds what a call may print.
const maxOutput = MaxRequestBytes + 1<<16

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

// wrapper is the sh program that applies Dir and Stdin, then execs the call.
const wrapper = `d=$1; shift
if [ "$d" != . ]; then cd "$d" || exit 126; fi
if [ -n "$NSELF_CI_STDIN" ]; then exec "$@" <"$NSELF_CI_STDIN"; fi
exec "$@" </dev/null`
