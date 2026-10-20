package economics

// Purpose: the pessimistic cost of a billed duration (Epic D11, review F5):
// owner-declared rate, billing increment, minimum charge and extras, rounded up
// to the next increment and to the next micro-unit, and never wrapped.
// Inputs:  a rate snapshot (store.RateSnap) and a duration in milliseconds.
// Outputs: Charge (micro-units and billed milliseconds, with a saturation flag)
// and Quote (the bound for a job timeout, with its status).
// Constraints: every product and sum goes through sched.MulDivUp, sched.Mul and
// sched.Add; a saturated figure is budget.exceeded and is never charged as a
// smaller number; a timeout above the cap is cost.unbounded; rates are only the
// owner's declaration (schedpolicy.Spend), never fetched.

import (
	"math"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Rate units and the increment used when none is declared.
const (
	msPerHour        = int64(3600 * 1000)
	msPerMinute      = int64(60 * 1000)
	defaultIncrement = msPerMinute
)

// Charge prices ms of run time under rs: whole increments, then the minimum
// charge and extras. A duration of zero or less bills nothing. overflow means
// a step saturated, and the returned figure is the saturated maximum.
func Charge(rs store.RateSnap, ms int64) (micro, billedMs int64, overflow bool) {
	if ms <= 0 {
		return 0, 0, false
	}
	inc := rs.IncMs
	if inc <= 0 {
		inc = defaultIncrement
	}
	units := ms / inc
	if ms%inc != 0 {
		units++
	}
	billed, o1 := sched.Mul(units, inc)
	micro, o2 := sched.MulDivUp(billed, rs.RateMicro, max(rs.RateUnitMs, 1))
	micro, o3 := sched.Add(micro, rs.MinMicro)
	micro, o4 := sched.Add(micro, rs.ExtrasMicro)
	if o1 || o2 || o3 || o4 {
		return math.MaxInt64, billed, true // a partial figure from a saturated step is not a smaller cost
	}
	return micro, billed, false
}

// SnapOf is the price snapshot of a declared rate; ok is false when the rate has neither a per-hour nor a per-minute price.
func SnapOf(r schedpolicy.Rate) (rs store.RateSnap, ok bool) {
	rs = store.RateSnap{IncMs: r.BillingIncrementMs, MinMicro: r.MinimumChargeMicro, ExtrasMicro: r.ExtrasMicro}
	switch {
	case r.PerHourMicro > 0:
		rs.RateMicro, rs.RateUnitMs = r.PerHourMicro, msPerHour
	case r.PerMinuteMicro > 0:
		rs.RateMicro, rs.RateUnitMs = r.PerMinuteMicro, msPerMinute
	default:
		return rs, false
	}
	if rs.IncMs <= 0 {
		rs.IncMs = defaultIncrement
	}
	return rs, true
}

// Quote is the worst-case cost of a job and why it can or cannot be used.
type Quote struct {
	Micro, BilledMs int64
	Snap            store.RateSnap
	// Reason is empty for a usable bound, else cost.unknown_rate, cost.unbounded or budget.exceeded (saturated).
	Reason model.PlacementReason
}

// QuoteJob bounds a job that may run timeoutMs under rate. capMs is the longest
// paid run allowed (economics.max_paid_timeout); zero means the default.
func QuoteJob(rate schedpolicy.Rate, timeoutMs, capMs int64) Quote {
	rs, ok := SnapOf(rate)
	if !ok {
		return Quote{Snap: rs, Reason: model.CostUnknownRate}
	}
	if capMs <= 0 {
		capMs = schedpolicy.DefaultMaxPaidTimeoutMs
	}
	if timeoutMs <= 0 || timeoutMs > capMs {
		return Quote{Snap: rs, Reason: model.CostUnbounded}
	}
	micro, billed, overflow := Charge(rs, timeoutMs)
	q := Quote{Micro: micro, BilledMs: billed, Snap: rs}
	if overflow {
		q.Reason = model.BudgetExceeded
	}
	return q
}
