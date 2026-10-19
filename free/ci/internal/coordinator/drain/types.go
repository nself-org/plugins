package drain

// Purpose: the vocabulary of the graceful drain (P7-NODE-20, Epic D7): the small interfaces the drain
//          needs from the coordinator (intake, leases, cancel delivery, clock) and the report it returns.
// Inputs:  none (declarations only).
// Outputs: Intake, Leases, Canceller, Clock, Lease, Config, Outcome and Report.
// Constraints: no import of agentapi or of the coordinator server: the agent API implements Canceller and
//          calls Drainer.Ack, the composition root passes the real intake and lease engine in. A job of
//          kind "release" or with idempotent false is never handed back (Lease.HandBackable).

import (
	"context"
	"time"
)

// DefaultDeadline is how long running leases may finish after SIGTERM before hand-back starts.
const DefaultDeadline = 10 * time.Minute

// Intake is the coordinator side that claims new work. Stop must be idempotent and must not return
// before no new claim can start.
type Intake interface{ Stop() }

// Canceller delivers a cancel to the agent that holds a lease (agentapi.Server implements it).
type Canceller interface {
	Cancel(nodeID, leaseID, reason string) error
}

// Clock is the time source; tests inject a fake. coordinator.Clock has the same shape.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Lease is one running lease as the drain sees it.
type Lease struct {
	ID, AttemptID, NodeID string
	Kind                  string // job kind: "release" is never handed back
	Idempotent            bool   // false: the job runs at most once
	Epoch                 int64
	ExpiresAt             time.Time
}

// HandBackable reports whether the lease may be cancelled and requeued.
func (l Lease) HandBackable() bool { return l.Kind != "release" && l.Idempotent }

// Leases is the lease side of the drain; EngineLeases adapts internal/lease (P7-NODE-09).
type Leases interface {
	// Running lists the leases of this coordinator that are still leased or running.
	Running(ctx context.Context) ([]Lease, error)
	// Get re-reads one lease; running is false once its attempt left leased and running.
	Get(ctx context.Context, id string) (l Lease, running bool, err error)
	// HandBack fences the lease and requeues its job. It is called only after a cancel ack
	// (expired false) or after the lease deadline (expired true).
	HandBack(ctx context.Context, l Lease, expired bool, reason string) error
	// NeedsOperator moves a lease that must not be requeued to needs-operator.
	NeedsOperator(ctx context.Context, l Lease, reason string) error
}

// Config configures a Drainer. Zero values take the defaults.
type Config struct {
	Intake     Intake
	Leases     Leases
	Canceller  Canceller
	Clock      Clock
	Deadline   time.Duration // how long running leases may finish (default DefaultDeadline)
	Poll       time.Duration // lease re-read interval (default 1 s)
	AckTimeout time.Duration // longest wait for an ack of a cancel while the lease keeps renewing (default 2 min)
	Logf       func(format string, a ...any)
}

// Outcome is what happened to one lease.
type Outcome string

const (
	// OutcomeFinished: the job ended on its own before or during the drain.
	OutcomeFinished Outcome = "finished"
	// OutcomeAcked: cancel acknowledged, then handed back.
	OutcomeAcked Outcome = "handed_back_after_ack"
	// OutcomeExpired: the lease expired after the cancel, then handed back.
	OutcomeExpired Outcome = "handed_back_after_expiry"
	// OutcomeNeedsOperator: a release or idempotent:false job; never handed back.
	OutcomeNeedsOperator Outcome = "needs_operator"
	// OutcomeLeftToGrace: no ack and the lease kept renewing; nothing was handed back, restart grace decides.
	OutcomeLeftToGrace Outcome = "left_to_grace"
	// OutcomeAbandoned: the context ended (hard stop) before the lease was settled.
	OutcomeAbandoned Outcome = "abandoned"
	// OutcomeError: a store or cancel failure; the lease was not handed back.
	OutcomeError Outcome = "error"
)

// Result is the settled outcome of one lease.
type Result struct {
	LeaseID string
	Outcome Outcome
	Err     error
}

// Report is the result of a drain. Leases are ordered by lease id.
type Report struct {
	Started, Ended time.Time
	Results        []Result
}

// Count returns how many leases ended with outcome o.
func (r Report) Count(o Outcome) int {
	n := 0
	for _, x := range r.Results {
		if x.Outcome == o {
			n++
		}
	}
	return n
}
