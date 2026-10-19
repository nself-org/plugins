package session

import (
	"context"
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// JobRun is everything a Runner needs for one lease.
type JobRun struct {
	Lease protocol.Lease
	// Slot is the index of the host slot the lease holds (the job UID derives from it).
	Slot  int
	Root  JobRoot
	Setup JobSetup
	// Credential is the lease's runner credential. The session zeroes it when the
	// lease ends: a Runner must not keep it or copy it into argv, a container
	// environment or a file.
	Credential []byte
	// Output receives the job's (already redacted) output chunks.
	Output func([]byte)
	// Tail returns the last part of the job's output as it left the node, with
	// the lease's secret values already masked: the source for the failure
	// excerpt in the evidence. It flushes an unterminated final line first.
	Tail func() string
	// Skip, when set, says the job must not run: "cancelled" (the lease was
	// cancelled or drained before it started) or "secrets_missing" (declared
	// secrets did not arrive before the lease expired). The Runner still returns
	// the result document for that outcome.
	Skip string
}

// JobResult is a finished job: its outcome and the ci.evidence/v1 document.
type JobResult struct {
	Outcome  Outcome
	Evidence json.RawMessage
}

// Runner runs one job. cmd/agent supplies the production one (internal/exec
// plus internal/evidence); tests supply scripted ones. A cancelled ctx must
// stop the job and still return its (cancelled) result.
type Runner interface {
	Run(ctx context.Context, job JobRun) (JobResult, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(context.Context, JobRun) (JobResult, error)

func (f RunnerFunc) Run(ctx context.Context, j JobRun) (JobResult, error) { return f(ctx, j) }
