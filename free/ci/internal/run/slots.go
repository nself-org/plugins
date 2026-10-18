package run

// Purpose: give each concurrent container attempt of one run its own execution slot, so the
// isolation profile derives a distinct job UID (65532 + slot, Epic D12).
// Inputs:  take/put around the executor call. Outputs: the lowest free slot.
// Constraints: slots are per coordinator run; cross-process slots come from the node's
// slot lease (P7-NODE-23), which sets JobSpec.Slot itself.

import "sync"

type slotPool struct {
	mu   sync.Mutex
	used map[int]bool
}

// take returns the lowest slot nobody holds.
func (p *slotPool) take() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.used == nil {
		p.used = map[int]bool{}
	}
	n := 0
	for p.used[n] {
		n++
	}
	p.used[n] = true
	return n
}

// put releases a slot.
func (p *slotPool) put(n int) {
	p.mu.Lock()
	delete(p.used, n)
	p.mu.Unlock()
}
