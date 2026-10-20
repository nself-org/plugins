package local

// Purpose: how many sandboxes this host may run at once for burst (Epic D13: capacity accounted
//          against the host reservation).
// Inputs:  the backend's Offer (the runtime's own limit, e.g. Apple's two VMs), the host capacity and
//          the node's stored reservation (internal/nodes/reservation), the job's CPU and memory need,
//          and the optional MaxSlots cap.
// Outputs: one integer, the smallest of the three limits; 0 means offer nothing now.
// Constraints: pure over its inputs apart from the host probe; a reservation the operator set is
//          never exceeded (the share is computed by the same function the node admission uses).

import (
	"math"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

func (p *Provider) limit(off Offer, s burst.Spec) int {
	lim := math.MaxInt
	if off.Total >= 0 {
		lim = off.Total
	}
	c, set := p.opts.Host()
	share := reservation.Compute(c, set, reservation.Need{CPU: float64(s.CPU), MemMB: int64(s.MemMB)})
	lim = min(lim, share.Slots)
	if p.opts.MaxSlots > 0 {
		lim = min(lim, p.opts.MaxSlots)
	}
	return lim
}
