package enroll

// Purpose: the per-source rate limit of the enrollment listener: one request per second per address.
// Inputs:  the source host and the current time.
// Outputs: Allow reports whether the request may proceed.
// Constraints: the table is bounded (entries older than a minute are dropped when it passes 4096), so a
//          spray of source addresses cannot grow memory without limit; the clock is injected.

import (
	"sync"
	"time"
)

// Interval is the minimum gap between two requests of one source.
const Interval = time.Second

const maxSources = 4096

// Limiter allows one request per Interval per source.
type Limiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter { return &Limiter{last: map[string]time.Time{}} }

// Allow records the request of src at now and reports whether it is within the limit.
func (l *Limiter) Allow(src string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if prev, ok := l.last[src]; ok && now.Sub(prev) < Interval {
		return false
	}
	if len(l.last) >= maxSources {
		for k, t := range l.last {
			if now.Sub(t) > time.Minute {
				delete(l.last, k)
			}
		}
	}
	l.last[src] = now
	return true
}
