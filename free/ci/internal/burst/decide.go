package burst

// Purpose: the wait-versus-provision decision (Epic D12), pure. Given the facts
// the caller read, Decide says whether one more machine may be created for a
// queued job, and why not when it may not.
// Inputs:  Input, which holds every fact the decision uses.
// Outputs: Decision{Allow, Reason, Latency, Input}: the record stores every input.
// Constraints: no os, os/exec, net or database/sql here (TestDecideIsPure); the
// answer depends on Input alone; Allow is true only when every rule holds, so a
// missing fact (a zero Enabled, PaidAllowed or ReservationFits) refuses; the
// first failing rule names the reason, in the fixed order of Rules; saturating
// arithmetic keeps a huge wait or latency from wrapping into an allow.

import (
	"math"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The decision reasons. BurstTeamOnly and PaidDisabled are the scheduler's own
// placement reasons; the rest belong to burst.
const (
	ReasonAllowed         = "provision"
	ReasonTeamOnly        = string(model.BurstTeamOnly)
	ReasonDisabled        = "provider.disabled"
	ReasonPaidDisabled    = string(model.PaidDisabled)
	ReasonTeardownFailed  = "teardown.failed"
	ReasonBreakerOpen     = "breaker.open"
	ReasonNoFit           = "reservation.no_fit"
	ReasonWaitShort       = "wait.short"
	ReasonInFlightCovers  = "inflight.covers"
	ReasonJobWindowCap    = "cap.job_window"
	ReasonProviderCap     = "cap.provider"
	DefaultLatencyMs      = int64(120 * 1000)
	DefaultMarginMs       = int64(60 * 1000)
	DefaultMaxConcurrent  = 2
	BreakerThreshold      = 3
	BreakerOpenMs         = int64(15 * 60 * 1000)
	jobWindowLatencyTimes = 2
)

// Input is every fact Decide uses; Decision stores it whole.
type Input struct {
	Provider string
	// Cloud is true for a provider that creates paid machines; Team is true on a TEAM coordinator.
	Cloud, Team bool
	// Enabled: the provider is enabled at operator scope. Paid: the provider's class is paid;
	// PaidAllowed: schedpolicy.PaidAllowed. ReservationFits: the lifetime reservation fits the budgets.
	Enabled, Paid, PaidAllowed, ReservationFits bool
	// ExpectedWaitMs is the queued job's expected wait; LatencyMs the provider's provisioning
	// latency (0 or less: unknown, DefaultLatencyMs); MarginMs the required margin.
	ExpectedWaitMs, LatencyMs, MarginMs int64
	// QueuedDemand counts the queued jobs the machine would serve; InFlight the provisions
	// already on their way (they count as capacity).
	QueuedDemand, InFlight int
	// JobWindowCount are provisions already started for this job within the window of
	// 2 x latency; Open the provider's intents that still hold money; MaxConcurrent its cap.
	JobWindowCount, Open, MaxConcurrent int
	// BreakerOpen: the join-failure breaker is open. TeardownFailed counts the provider's
	// failed teardowns not yet confirmed gone.
	BreakerOpen    bool
	TeardownFailed int
	// FitReason says why the reservation does not fit (a record only; ReservationFits decides).
	FitReason string
}

// Decision is the answer and the record of how it was reached.
type Decision struct {
	Allow  bool
	Reason string
	// Latency is the latency used (the provider's, or the default when unknown).
	Latency int64
	Input   Input
}

// EffectiveLatency is the latency Decide uses: unknown (zero or less) is 120 s.
func EffectiveLatency(ms int64) int64 {
	if ms <= 0 {
		return DefaultLatencyMs
	}
	return ms
}

// WindowMs is the per-job window: 2 x the effective latency.
func WindowMs(latencyMs int64) int64 {
	return satAdd(EffectiveLatency(latencyMs), EffectiveLatency(latencyMs))
}

func satAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// Decide applies the rules of Epic D12 in order and returns the first that refuses.
func Decide(in Input) Decision {
	lat := EffectiveLatency(in.LatencyMs)
	d := Decision{Reason: ReasonAllowed, Latency: lat, Input: in}
	refuse := func(reason string) Decision { d.Allow, d.Reason = false, reason; return d }
	switch {
	case in.Cloud && !in.Team:
		return refuse(ReasonTeamOnly)
	case !in.Enabled:
		return refuse(ReasonDisabled)
	case in.Paid && !in.PaidAllowed:
		return refuse(ReasonPaidDisabled)
	case in.TeardownFailed > 0:
		return refuse(ReasonTeardownFailed)
	case in.BreakerOpen:
		return refuse(ReasonBreakerOpen)
	case !in.ReservationFits:
		return refuse(ReasonNoFit)
	case in.ExpectedWaitMs <= satAdd(lat, max(in.MarginMs, 0)):
		return refuse(ReasonWaitShort)
	case in.QueuedDemand <= in.InFlight:
		return refuse(ReasonInFlightCovers)
	case in.JobWindowCount >= 1:
		return refuse(ReasonJobWindowCap)
	case in.Open >= max(in.MaxConcurrent, 0):
		return refuse(ReasonProviderCap)
	}
	d.Allow = true
	return d
}
