package dispatch

// Purpose: the DecisionSink hook P7-SCHED-05 fills (decision records and outcome
// telemetry). The dispatcher calls it for every placement it admits or refuses,
// with the digest of the world the placement was computed from.
// Constraints: the default is a no-op; a sink must not block the tick.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// Outcome values of a Decision.
const (
	OutcomeAdmitted     = "admitted"
	OutcomeRefused      = "refused"
	OutcomeConflict     = "conflict"
	OutcomeWaiting      = "waiting"
	OutcomeUnplaceable  = "unplaceable"
	OutcomeTimeout      = "timeout"
	OutcomeFailed       = "failed"
	OutcomeUnknownKind  = "unknown_transport"
	OutcomeAwaitingAppr = "awaiting-approval"
)

// Decision is one placement and what admission did with it.
type Decision struct {
	WorldDigest string
	Job         sched.Job
	Placement   *sched.Placement // nil when nothing was placed
	Explanation sched.Explanation
	Outcome     string
	Reason      model.PlacementReason // the refusal reason, when there is one
}

// DecisionSink receives decisions.
type DecisionSink interface {
	Record(context.Context, Decision)
}

type noSink struct{}

func (noSink) Record(context.Context, Decision) {}
