package run

// Purpose: what the remote sink does with a result that asks for a re-run instead of a verdict
// (D-1103): an infra failure the node reports, or a cancel nobody asked for (a drain, an agent
// self-cancel). Such an attempt is lost, not finalizing, so the CI-40 infra retry can queue it
// again; finalizing has no path back to the queue.
// Inputs:  the node's checked evidence and its protocol result.
// Outputs: the attempt moved to lost, and the run told through the carrier-lost channel.
// Constraints: the lease epoch is fenced first, as for any result. A result frame that carries a
// signature is verified against the node registry (attest.VerifyFrame); one the key does not verify is
// an infra failure of the node, never a verdict. An unsigned frame is accepted as before: the agent
// contract (P7-TRUST-20) lets a node without the signing-eligible marker complete unsigned, and the
// pinned ssh channel authenticates it. The release gate still refuses unsigned remote evidence.

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// nodeReported is the cause of a lost attempt whose node said so itself. A reported infra
// failure counts against the node's breaker; a cancel the node chose does not.
type nodeReported struct {
	msg   string
	infra bool
}

func (e *nodeReported) Error() string { return e.msg }

var (
	errNodeInfra     = &nodeReported{msg: "the node reported an infrastructure failure", infra: true}
	errNodeCancelled = &nodeReported{msg: "the node cancelled the job without being asked"}
)

// signatureCause is why a signed frame must not be trusted, or nil for an unsigned frame or one the
// registry key verifies.
func (s *remoteSink) signatureCause(r protocol.Result) error {
	if r.Signature.KeyID == "" && r.Signature.Value == "" {
		return nil
	}
	led := func() attest.Ledger { return store.AttestLedger{S: s.st} }
	if err := attest.VerifyFrame(led, r.Evidence, r.Signature.KeyID, r.Signature.Value); err != nil {
		return &nodeReported{msg: "the node's result signature was refused: " + string(attest.RemoteReasonOf(err)), infra: true}
	}
	return nil
}

// evidenceFailure is the failure class and exit code a non-pass evidence document settles to: an
// error result is infra and a check's own class overrides, the last check that names one winning.
func evidenceFailure(ev model.Evidence) (class model.FailureClass, exit int) {
	class, exit = "code", 1
	if ev.Result == "error" {
		class = "infra"
	}
	for _, c := range ev.Checks {
		if c.FailureClass != "" {
			class = c.FailureClass
		}
		if c.Exit != nil {
			exit = *c.Exit
		}
	}
	return class, exit
}

// unsettledCause is why the attempt must be re-run instead of settled from ev, or nil. A cancel the
// run asked for is the run's to settle.
func (ra *remoteAttempt) unsettledCause(ev model.Evidence) error {
	switch ev.Result {
	case "pass":
		return nil
	case "cancelled":
		ra.mu.Lock()
		asked := ra.cancelled
		ra.mu.Unlock()
		if asked {
			return nil
		}
		return errNodeCancelled
	}
	if class, _ := evidenceFailure(ev); class == "infra" {
		return errNodeInfra
	}
	return nil
}

// failLease fails the lease of a fenced result: the attempt becomes lost (as when a carrier ends)
// and the run is told, so remoteLost parks it for the retry.
func (s *remoteSink) failLease(ctx context.Context, ra *remoteAttempt, r protocol.Result, cause error) error {
	if err := s.eng.Fence(ctx, r.LeaseID, int64(r.Epoch)); err != nil {
		return err
	}
	state, epoch, err := s.st.AttemptStateEpoch(ctx, ra.attemptID)
	if err != nil {
		return err
	}
	if state != "running" {
		return fail(CodeRun, "result for lease %s while the attempt is %s", r.LeaseID, state)
	}
	if err = s.st.Transition(ctx, ra.attemptID, state, "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "infra", Reason: cause.Error()}); err != nil {
		return err
	}
	s.ended(ra.attemptID, cause)
	return nil
}

// reportedByNode says whether err is a loss the node reported itself.
func reportedByNode(err error) (*nodeReported, bool) {
	var n *nodeReported
	return n, errors.As(err, &n)
}
