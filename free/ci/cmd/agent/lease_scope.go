package main

// Purpose: the network scope a leased job runs under (D-1331).
// Inputs:  the lease's trust class and declared scope (execSpec.Trust, execSpec.Network).
// Outputs: the effective scope for ciexec.JobSpec.Network, or an E679 refusal as an attempt result.
// Constraints: a job with no trust class is not scoped (the owner-only `exec` command keeps its none default);
// a lease always carries one, because an absent class is read as untrusted. The scope is never raised: an
// untrusted job that declares lan or privileged is refused here, whatever the coordinator admitted.

import (
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// jobScope returns the scope spec runs under; "" means the executor default (none).
func jobScope(spec execSpec) (model.NetworkScope, error) {
	if spec.Trust == "" && spec.Network == "" {
		return "", nil
	}
	return netscope.Effective(spec.Trust, spec.Network)
}

// leaseSpec is the job spec of a lease. An absent trust class is read as untrusted (trust.DecodeTrust), so the
// scope check always runs for a lease.
func leaseSpec(l protocol.Lease, coordinator, sourceDir string) execSpec {
	return execSpec{AttemptID: l.AttemptID, CoordinatorID: coordinator, JobID: "job", Job: l.Job, Revision: l.Checkout.Revision, SourceDir: sourceDir,
		Trust: trust.DecodeTrust(string(l.Trust)), Network: l.Network}
}

// refusedScope is the attempt result of a job whose scope was refused: nothing ran.
func refusedScope(attemptID string, err error) ciexec.AttemptResult {
	now := time.Now()
	return ciexec.AttemptResult{Attempt: model.Attempt{ID: attemptID, State: "failed", FailureClass: "infra"}, ExitCode: -1, Started: now, Finished: now,
		Err: &ciexec.Error{Code: netscope.CodeScopeFailed, Message: err.Error()}}
}
