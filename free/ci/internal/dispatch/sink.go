package dispatch

// Purpose: the DecisionSink hook P7-SCHED-05 fills (decision records and outcome
// telemetry). The dispatcher calls it for every placement it admits or refuses,
// with the digest of the world the placement was computed from.
// Constraints: the default is a no-op unless a SinkFactory is registered; a
// sink must not block the tick.

import (
	"context"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
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
	OutcomeRetry        = "retry"
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
	// World is the world the placement was computed from (nil for a timeout,
	// which is decided before placement). It is valid for the Record call only:
	// a sink reads it and keeps no reference.
	World *sched.World
}

// DecisionSink receives decisions.
type DecisionSink interface {
	Record(context.Context, Decision)
}

type noSink struct{}

func (noSink) Record(context.Context, Decision) {}

// SinkFactory builds the DecisionSink of one dispatcher from its store and host.
type SinkFactory func(st *store.Store, host string) DecisionSink

var (
	sinkMu      sync.Mutex
	sinkFactory SinkFactory
)

// RegisterSinkFactory installs the factory New uses when Config.Sink is nil
// (P7-SCHED-05 registers the telemetry sink from its init). One factory only.
func RegisterSinkFactory(f SinkFactory) error {
	if f == nil {
		return fmt.Errorf("dispatch: a sink factory needs a function")
	}
	sinkMu.Lock()
	defer sinkMu.Unlock()
	if sinkFactory != nil {
		return fmt.Errorf("dispatch: a sink factory is already registered")
	}
	sinkFactory = f
	return nil
}

// defaultSink is the registered factory's sink, or the no-op sink.
func defaultSink(cfg Config) DecisionSink {
	sinkMu.Lock()
	f := sinkFactory
	sinkMu.Unlock()
	if f != nil {
		if s := f(cfg.Store, cfg.Host); s != nil {
			return s
		}
	}
	return noSink{}
}
