package local

// Purpose: provisioning latency per isolation level, measured and fed to Estimate (Epic D13).
// Inputs:  Observe(level, duration) calls: the time from a granted slot to the first command
//          running in its sandbox, reported by whoever starts the job (and by the integration tests).
// Outputs: the smoothed latency in milliseconds per level, or the level's prior until one
//          sample exists.
// Constraints: priors are conservative guesses and are replaced by the first measurement;
//          the smoothing weight is fixed (1/4 new sample); a zero or negative sample is ignored.

import (
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// priors are used until a level has a measurement: a hardened container starts in seconds, gVisor adds
// its sentry start, and a Tart clone boots a macOS guest.
var priors = map[model.Isolation]int64{
	containerLevel: 3_000,
	sandboxLevel:   6_000,
	vmLevel:        60_000,
}

type latencies struct {
	mu  sync.Mutex
	ms  map[model.Isolation]int64
	cnt map[model.Isolation]int
}

func newLatencies() *latencies {
	return &latencies{ms: map[model.Isolation]int64{}, cnt: map[model.Isolation]int{}}
}

func (l *latencies) observe(level model.Isolation, d time.Duration) {
	if d <= 0 {
		return
	}
	ms := max(1, d.Milliseconds())
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cnt[level] == 0 {
		l.ms[level] = ms
	} else {
		l.ms[level] = (3*l.ms[level] + ms) / 4
	}
	l.cnt[level]++
}

// get returns the latency estimate and how many samples back it (0 = the prior).
func (l *latencies) get(level model.Isolation) (ms int64, samples int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cnt[level] > 0 {
		return l.ms[level], l.cnt[level]
	}
	return priors[level], 0
}
