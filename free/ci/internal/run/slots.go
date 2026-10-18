package run

// Purpose: give each concurrent container attempt its own execution slot, so the isolation
// profile derives a distinct job UID (65532 + slot, Epic D12), across coordinators too.
// Inputs:  take(dir)/put(n) around the executor call; dir is <home>/slots.
// Outputs: the lowest slot nobody holds, in this process or in another one using the same home.
// Constraints: another process's hold is an advisory flock on <dir>/slot-<n>.lock (released by
// the kernel if that process dies); where locking is unavailable the slot is process-local.
// The node's slot lease (P7-NODE-23) supersedes this by setting JobSpec.Slot itself.

import (
	"os"
	"path/filepath"
	"sync"
)

type slotPool struct {
	mu   sync.Mutex
	held map[int]*os.File // nil file: held in this process only
}

// slotDir is where the slot locks live for a run home.
func slotDir(home string) string {
	if home == "" {
		home = os.Getenv("NSELF_CI_HOME")
	}
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return filepath.Join(home, "slots")
}

// maxSlots bounds the search; take returns -1 (JobUID refuses it, so the attempt fails as
// infra) when every slot is held or its lock is unusable.
const maxSlots = 256

// take returns the lowest slot nobody holds, or -1.
func (p *slotPool) take(dir string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held == nil {
		p.held = map[int]*os.File{}
	}
	for n := 0; n < maxSlots; n++ {
		if _, mine := p.held[n]; mine {
			continue
		}
		if f, ok := lockSlot(dir, n); ok {
			p.held[n] = f
			return n
		}
	}
	return -1
}

// put releases a slot.
func (p *slotPool) put(n int) {
	if n < 0 {
		return
	}
	p.mu.Lock()
	f := p.held[n]
	delete(p.held, n)
	p.mu.Unlock()
	if f != nil {
		_ = f.Close() // closing drops the flock
	}
}
