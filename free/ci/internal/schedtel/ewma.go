// Package schedtel records scheduling decisions and outcomes and turns them
// into runtime, cold-start, wait and reliability estimates for the placement
// world (P7-SCHED-05, Epic D16).
//
// Purpose: local scheduling telemetry; no ML, no network, no ping_api (X15).
// Inputs: dispatch decisions (sink), finished runs (finalizer), the store.
// Outputs: decision records, outcome rows, EWMA estimators, the `schedtel`
// world contributor (field Estimates), the planview DecisionSource.
// Constraints: integer arithmetic only; an estimate is unknown below
// MinSamples samples and is then not offered to the scheduler at all.
package schedtel

import "github.com/nself-org/plugins/free/ci/internal/store"

// Estimator constants (Epic D16).
const (
	// AlphaNum/AlphaDen is the EWMA weight of the newest sample (3/10).
	AlphaNum, AlphaDen = 3, 10
	// MinSamples is the number of samples at which confidence leaves unknown.
	MinSamples = 3
	// ReliabilityWindow is how many of a runner's latest attempts count.
	ReliabilityWindow = 20
	// RefPrefix starts the placement_ref of an attempt: RefPrefix + attempt id.
	RefPrefix = "sched:decision/"
)

// Confidence names how much a sample count supports an estimate.
func Confidence(n int64) string {
	if n < MinSamples {
		return "unknown"
	}
	return "estimated"
}

// ewma folds sample into old after n previous samples: the first sample is
// the value, then new = (3*sample + 7*old) / 10 rounded half up.
func ewma(old, sample, n int64) int64 {
	if n <= 0 {
		return sample
	}
	return (AlphaNum*sample + (AlphaDen-AlphaNum)*old + AlphaDen/2) / AlphaDen
}

// fold returns the estimator update for one outcome: runtimeMs < 0 and
// coldMs < 0 mean "not measured" and leave that estimate alone.
func fold(runtimeMs, coldMs, now int64) func(store.TelEstimator, bool) store.TelEstimator {
	return func(cur store.TelEstimator, _ bool) store.TelEstimator {
		if runtimeMs >= 0 {
			cur.RuntimeMs, cur.N = ewma(cur.RuntimeMs, runtimeMs, cur.N), cur.N+1
		}
		if coldMs >= 0 {
			cur.ColdStartMs, cur.ColdN = ewma(cur.ColdStartMs, coldMs, cur.ColdN), cur.ColdN+1
		}
		cur.Updated = now
		return cur
	}
}
