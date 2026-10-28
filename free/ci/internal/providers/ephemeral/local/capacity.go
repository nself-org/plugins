package local

// Purpose: how many sandboxes this host may run at once for burst (Epic D13: capacity accounted
//          against the host reservation), and how many it holds now (used).
// Inputs:  the backend's Offer (the runtime's own limit, e.g. Apple's two VMs), the host capacity (with its free
//          memory now) and the node's stored reservation (internal/nodes/reservation), the regular lanes running
//          now, the job's CPU and memory need, and the optional MaxSlots cap.
// Outputs: one integer, the smallest of the limits; 0 means offer nothing now.
// Constraints: pure over its inputs apart from the host probe; a reservation the operator set is
//          never exceeded (the share is computed by the same function the node admission uses).

import (
	"context"
	"math"
	"strings"
	"time"

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

// memorySettle is how long a granted slot may take to start using its memory.
const memorySettle = time.Minute

// limit is how many sandboxes burst may hold at once: the smallest of the runtime's own limit, the host-reservation
// share less the slots regular lane jobs hold now, what the host's free memory affords on top of the sandboxes already
// running (used), and MaxSlots. The node's load is read live: a static share would grant slots the lanes hold.
func (p *Provider) limit(off Offer, s burst.Spec, used int) int {
	lim := math.MaxInt
	if off.Total >= 0 {
		lim = off.Total
	}
	c, set := p.opts.Host()
	need := reservation.Charge(c, set, reservation.Need{CPU: float64(s.CPU), MemMB: int64(s.MemMB)})
	share := reservation.Compute(c, set, need)
	lanes := 0
	if p.opts.Lanes != nil {
		lanes = max(0, p.opts.Lanes())
	}
	lim = min(lim, max(0, share.Slots-lanes))
	if need.MemMB > 0 && c.FreeMemoryMB > 0 {
		// free memory already excludes the sandboxes and lanes running, but not a slot just granted whose job has
		// not started: those still claim their share of it
		room := max(0, min(int64(c.FreeMemoryMB)/need.MemMB, math.MaxInt32)-int64(p.unsettled()))
		lim = min(lim, used+int(room))
	}
	if p.opts.MaxSlots > 0 {
		lim = min(lim, p.opts.MaxSlots)
	}
	return lim
}

// unsettled counts the slots granted too recently for their memory to show in the host's free memory: not yet
// reported started and younger than memorySettle.
func (p *Provider) unsettled() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	now, n := p.opts.Now(), 0
	for _, e := range p.ledger {
		if !e.started && now.Sub(e.res.CreatedAt) < memorySettle {
			n++
		}
	}
	return n
}

func (p *Provider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ledger)
}

// used is the slots taken now: every reservation, plus every sandbox the runtime holds that no reservation of
// this provider explains (a leak, another process's job, a VM Apple's limit counts).
func (p *Provider) used(off Offer, found []Found) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := map[string]bool{}
	for _, f := range found {
		if !strings.HasSuffix(f.ID, cleanSuffix) { // a job's cleanup helper is not a second slot
			seen[f.ID] = true
		}
	}
	for _, id := range off.Running {
		seen[id] = true
	}
	n := len(p.ledger)
	for id := range seen {
		if _, ok := p.ledger[id]; !ok {
			n++
		}
	}
	return n
}
