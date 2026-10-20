package agent

// Purpose: the reconnect delay of the pull client: exponential from 1 s to 60 s with full jitter, so a fleet of
//          agents that lost one coordinator does not come back in step.
// Inputs:  Next per failed attempt, Reset after a healthy session.
// Outputs: the delay to wait before the next attempt.
// Constraints: the delay is uniform in [floor, cap) where cap doubles per attempt up to Max; the random source
//          and the bounds are fields so tests are exact.

import (
	"math/rand/v2"
	"time"
)

// Default bounds (Epic D6, ticket constraint).
// maxAttempts stops the attempt counter (and the doubling loop) growing without end.
const maxAttempts = 64

const (
	BackoffMin   = time.Second
	BackoffMax   = 60 * time.Second
	backoffFloor = 50 * time.Millisecond
)

// Backoff computes reconnect delays. The zero value uses the defaults and the global random source.
type Backoff struct {
	Min, Max time.Duration
	// Rand returns a float64 in [0, 1); nil uses math/rand/v2.
	Rand func() float64

	attempt int
}

func (b *Backoff) bounds() (lo, hi time.Duration) {
	lo, hi = b.Min, b.Max
	if lo <= 0 {
		lo = BackoffMin
	}
	if hi < lo {
		hi = BackoffMax
	}
	return lo, hi
}

// Next returns the delay after one more failure.
func (b *Backoff) Next() time.Duration {
	lo, hi := b.bounds()
	ceiling := lo
	for i := 0; i < b.attempt; i++ {
		ceiling = min(ceiling*2, hi)
	}
	b.attempt = min(b.attempt+1, maxAttempts)
	r := b.Rand
	if r == nil {
		r = rand.Float64
	}
	d := time.Duration(r() * float64(ceiling))
	return max(d, min(backoffFloor, ceiling))
}

// Reset starts over after a healthy session.
func (b *Backoff) Reset() { b.attempt = 0 }
