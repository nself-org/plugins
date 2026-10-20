package report

// Purpose: the retry delay of a failed status delivery (Epic D7): 30 s x 2^n,
// capped at 10 minutes, with +-20% jitter.
// Inputs:  n failed attempts so far, a random value in [0,1).
// Outputs: the wait before the next attempt.
// Constraints: pure; the wait never exceeds the cap. A jittered wait above the
// cap is folded back below it, so entries that have reached the cap still
// spread over [0.8 cap, cap] instead of all retrying at the same instant.

import "time"

// Backoff constants.
const (
	BackoffBase   = 30 * time.Second
	BackoffCap    = 10 * time.Minute
	BackoffJitter = 0.2
)

// Backoff is the wait after n earlier failures (0: the first failure) for a
// random value rnd in [0,1).
func Backoff(n int, rnd float64) time.Duration {
	d := BackoffBase
	for i := 0; i < n && d < BackoffCap; i++ {
		d *= 2
	}
	d = time.Duration(float64(min(d, BackoffCap)) * (1 + (2*rnd-1)*BackoffJitter))
	if d > BackoffCap {
		d = 2*BackoffCap - d
	}
	return d
}
