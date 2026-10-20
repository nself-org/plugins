package health

// Purpose: the per-provider circuit breaker of Epic D13 (P7-HOST-11, cap:ci.provider-health). It
//          turns observed API outcomes into a health state and nothing else: no status page is read
//          and no reachability probe is sent (Constitution §8.2).
// Inputs:  rest.Outcome values (class, HTTP status, RetryAt) and an injected clock.
// Outputs: a providers.Health (up | degraded | down) and a Snapshot the tracker persists.
// Constraints: down after DownAfter consecutive transient or outage outcomes within DownWindow, or
//          after 5xx answers for ServerSpan with no answer in between; degraded while a quota
//          outcome's RetryAt (or QuotaPause when it carries none) is in the future, and the breaker
//          never opens for a quota answer; after the probe gap (ProbeAfter, doubling to ProbeMax on
//          each failed probe) a down breaker reads degraded/provider.probing, so placement may send
//          a request, and one reachable answer closes it. An answer is an HTTP response with a
//          non-5xx status (an auth or config refusal included): it proves the provider is
//          reachable, so it ends an outage but is not a success for placement. A failure that sent
//          no request (a missing credential, a refused URL: Status 0, class other than transient or
//          outage) says nothing about the provider and is ignored. The 5xx rule needs three 5xx
//          answers, no two more than ServerSpan apart. Health is a pure read: it never claims a
//          probe slot, so a display path cannot use one up. A zero-value Breaker is not usable.

import (
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// Thresholds of D13.
const (
	DownAfter  = 5                // consecutive failures that open the breaker
	DownWindow = 2 * time.Minute  // the failures must fall within this span
	ServerSpan = 60 * time.Second // 5xx answers for this long open it too
	ProbeAfter = 30 * time.Second // first probe gap
	ProbeMax   = 10 * time.Minute // the probe gap stops doubling here
	QuotaPause = time.Minute      // degraded span of a quota answer that names no RetryAt
	ServerRun  = 3                // 5xx answers the sixty second rule needs
)

// Reasons carried in providers.Health.Reason.
const (
	ReasonDown    = "provider.down"
	ReasonQuota   = "provider.rate_limited"
	ReasonProbing = "provider.probing"
)

// Snapshot is the durable part of a breaker. It holds no credential and no response data.
type Snapshot struct {
	Down          bool          `json:"down"`
	DownSince     time.Time     `json:"down_since,omitempty"`
	NextProbe     time.Time     `json:"next_probe,omitempty"`
	ProbeGap      time.Duration `json:"probe_gap,omitempty"`
	DegradedSince time.Time     `json:"degraded_since,omitempty"`
	DegradedUntil time.Time     `json:"degraded_until,omitempty"`
}

// Breaker is the health state machine of one provider API.
type Breaker struct {
	mu      sync.Mutex
	snap    Snapshot
	fails   []time.Time // the current run of consecutive failures, newest last, at most DownAfter
	server  time.Time   // first 5xx of the unbroken run of 5xx answers; zero when none
	last5xx time.Time   // the newest 5xx of that run
	servers int         // 5xx answers in that run
	seen    bool        // this process observed an outcome: its state is authoritative
}

// NewBreaker returns a closed breaker.
func NewBreaker() *Breaker { return &Breaker{} }

// Observe feeds one outcome. It reports whether the breaker moved from down to up and whether its
// durable state changed (the tracker persists on a change).
func (b *Breaker) Observe(o rest.Outcome, now time.Time) (recovered, changed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seen = true
	before := b.snap
	switch {
	case o.Class == providers.Transient || o.Class == providers.Outage:
		b.fail(o, now)
	case o.Status <= 0: // no response came back and nothing was sent: not evidence either way
	case o.Class == providers.Quota:
		recovered = b.answer()
		b.quota(o, now)
	default:
		recovered = b.answer()
	}
	return recovered, b.snap != before
}

// fail records a transient or outage outcome.
func (b *Breaker) fail(o rest.Outcome, now time.Time) {
	if b.snap.Down {
		if !now.Before(b.snap.NextProbe) { // a probe (or a late one) failed: wait twice as long
			b.snap.ProbeGap = min(max(b.snap.ProbeGap, ProbeAfter)*2, ProbeMax)
			b.snap.NextProbe = now.Add(b.snap.ProbeGap)
		}
		return
	}
	if n := len(b.fails); n > 0 && now.Sub(b.fails[n-1]) > DownWindow { // a stale run is not a streak
		b.fails = nil
	}
	if len(b.fails) == DownAfter {
		b.fails = b.fails[1:]
	}
	b.fails = append(b.fails, now)
	server := o.Status >= 500
	if server {
		if b.servers == 0 || now.Sub(b.last5xx) > ServerSpan { // a gap longer than the span breaks the run
			b.server, b.servers = now, 0
		}
		b.last5xx, b.servers = now, b.servers+1
	}
	streak := len(b.fails) == DownAfter && now.Sub(b.fails[0]) <= DownWindow
	long := server && b.servers >= ServerRun && now.Sub(b.server) >= ServerSpan
	if streak || long {
		b.snap.Down, b.snap.DownSince, b.snap.ProbeGap = true, now, ProbeAfter
		b.snap.NextProbe = now.Add(ProbeAfter)
	}
}

// answer records an answer that proves the provider is reachable; it reports a down -> up move.
func (b *Breaker) answer() (recovered bool) {
	b.fails, b.server, b.last5xx, b.servers = nil, time.Time{}, time.Time{}, 0
	if b.snap.Down {
		b.snap.Down, b.snap.DownSince, b.snap.NextProbe, b.snap.ProbeGap = false, time.Time{}, time.Time{}, 0
		return true
	}
	return false
}

// quota marks the provider degraded until the answer's RetryAt.
func (b *Breaker) quota(o rest.Outcome, now time.Time) {
	until := o.RetryAt
	if until.IsZero() {
		until = now.Add(QuotaPause)
	}
	if !until.After(now) {
		return // the window already ended: nothing to wait for
	}
	if !b.snap.DegradedUntil.After(now) {
		b.snap.DegradedSince = now
	}
	if until.After(b.snap.DegradedUntil) {
		b.snap.DegradedUntil = until
	}
}

// Health is the state at now. It is a pure read. A down breaker whose probe gap has passed reads
// degraded with reason provider.probing: placement may send a request, and the first outcome ends
// the window (a failure doubles the gap, an answer closes the breaker).
func (b *Breaker) Health(now time.Time) providers.Health {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.snap
	switch {
	case s.Down && now.Before(s.NextProbe):
		return providers.Health{State: "down", Reason: ReasonDown, Since: s.DownSince, RetryAt: s.NextProbe}
	case s.Down:
		return providers.Health{State: "degraded", Reason: ReasonProbing, Since: s.DownSince, RetryAt: s.NextProbe}
	case s.DegradedUntil.After(now):
		return providers.Health{State: "degraded", Reason: ReasonQuota, Since: s.DegradedSince, RetryAt: s.DegradedUntil}
	}
	return providers.Health{State: "up"}
}

// Snapshot returns the durable state.
func (b *Breaker) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snap
}

// Restore loads a persisted state into a breaker that has seen no outcome in this process.
// It reports whether the state was taken.
func (b *Breaker) Restore(s Snapshot) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen {
		return false
	}
	b.snap = s
	return true
}

// Observed reports whether this process has fed the breaker an outcome.
func (b *Breaker) Observed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seen
}
