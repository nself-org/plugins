package hostedtier

// Purpose: the Breaker port over the HOST-11 tracker (P7-AUTO-07): billing-blocked is recorded as
//          a quota-class outcome, which the breaker reads as unavailable until RetryAt.
// Inputs:  a *health.Tracker and the hosted provider id.
// Outputs: NewHealthBreaker.
// Constraints: no new state of its own; the tracker owns persistence.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

type trackerBreaker struct {
	t  *health.Tracker
	id string
}

// NewHealthBreaker returns the Breaker of provider id backed by t.
func NewHealthBreaker(t *health.Tracker, id string) Breaker { return trackerBreaker{t: t, id: id} }

// Blocked is true while the breaker is down or paused for a limit; a probing breaker lets a
// request through, whose outcome ends the window.
func (b trackerBreaker) Blocked(ctx context.Context) bool {
	h := b.t.HealthOf(ctx, b.id)
	return h.State == "down" || h.State == "degraded" && h.Reason == health.ReasonQuota
}

// MarkBlocked records billing-blocked as a quota outcome that lasts until until.
func (b trackerBreaker) MarkBlocked(until time.Time) {
	b.t.Observe(rest.Outcome{Provider: b.id, Class: providers.Quota, Status: 403, RetryAt: until})
}
