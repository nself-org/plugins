package local

// Purpose: how many sandboxes this host may run at once for burst (Epic D13: capacity accounted
//          against the host reservation).
// Inputs:  the backend's Offer (the runtime's own limit, e.g. Apple's two VMs), the host capacity and
//          the node's stored reservation (internal/nodes/reservation), the job's CPU and memory need,
//          and the optional MaxSlots cap.
// Outputs: one integer, the smallest of the three limits; 0 means offer nothing now.
// Constraints: pure over its inputs apart from the host probe and the lane reader; a reservation the operator set is
//          never exceeded (the share is computed by the same function the node admission uses).

import (
	"context"
	"math"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

// reclaim asks a backend that sweeps by its own proof (Tart) to remove dead owners' sandboxes; it reports whether
// the backend can, so the caller re-reads capacity only when something may have changed.
func (p *Provider) reclaim(ctx context.Context) bool {
	r, ok := p.opts.Backend.(Reclaimer)
	if ok {
		r.Reclaim(ctx)
	}
	return ok
}

// limit is how many sandboxes may exist at once: the smallest of the runtime limit, the host-reservation share less the
// regular lane jobs running now, the slots the node's free memory still holds (a sandbox already running is in
// used, so only the memory it has not taken yet counts), and MaxSlots.
func (p *Provider) limit(off Offer, s burst.Spec, used int) int {
	lim := math.MaxInt
	if off.Total >= 0 {
		lim = off.Total
	}
	c, set := p.opts.Host()
	need := reservation.Need{CPU: float64(s.CPU), MemMB: int64(s.MemMB)}
	share := reservation.Compute(c, set, need)
	lim = min(lim, max(0, share.Slots-p.lanes()))
	if need.MemMB > 0 && c.FreeMemoryMB > 0 { // an unset reading (0) is unknown, not empty
		lim = min(lim, used+int(int64(c.FreeMemoryMB)/need.MemMB))
	}
	if p.opts.MaxSlots > 0 {
		lim = min(lim, p.opts.MaxSlots)
	}
	return lim
}

// lanes is the number of regular (non-burst) lane jobs running on this node now; 0 until a reader is wired.
func (p *Provider) lanes() int {
	if p.opts.Lanes == nil {
		return 0
	}
	return max(0, p.opts.Lanes())
}
