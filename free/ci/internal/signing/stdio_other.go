//go:build !unix

package signing

// Purpose: the non-unix FunnelRunner: refuses, because the stdin FIFO needs
// mkfifo and /bin/sh (the signer and `key init` are unix-only in v1.5).
// Inputs: a StdioCall. Outputs: an E717 signer_failed refusal.
// Constraints: nothing is run.

import "context"

// FunnelRunner is the production StdioRunner (unix only).
type FunnelRunner struct {
	// Home is the CI state directory the funnel works under.
	Home string
}

// RunStdio implements StdioRunner.
func (r *FunnelRunner) RunStdio(context.Context, StdioCall) ([]byte, int, error) {
	return nil, notRun, Failf(ReasonSignerFailed, "the release signer needs a unix host")
}
