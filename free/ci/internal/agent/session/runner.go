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
