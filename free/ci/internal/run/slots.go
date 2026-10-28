package run

// Purpose: give each concurrent container attempt its own execution slot, so the isolation
// profile derives a distinct job UID (65532 + slot, Epic D12), across coordinators too.
// Inputs: take(dir)/put(n) around the executor call; Linux uses host-wide leases.
// Outputs: the lowest unheld slot, shared across Linux coordinators and OS users.
// Constraints: Linux uses abstract Unix sockets in the host network namespace;
// other Unix platforms use advisory flock files under the home. Both are released
// by the kernel if the holder dies; Windows remains process-local.
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

// resolveCIHome returns the CI state directory: Options.Home, else NSELF_CI_HOME,
// else ~/.nself/ci. It is independent of slot allocation so hygiene targets the
// real workspace even on Linux, where slots live in an abstract socket namespace.
func resolveCIHome(home string) string {
	if home != "" {
		return home
	}
	if home = os.Getenv("NSELF_CI_HOME"); home != "" {
		return home
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(user, ".nself", "ci")
}

// slotDir selects the host namespace on Linux, otherwise the run home locks.
// It must NOT be used to derive the CI home; use resolveCIHome for that.
func slotDir(home string) string {
	if dir := hostSlots(); dir != "" {
		return dir
	}
	h := resolveCIHome(home)
	if h == "" {
		return ""
	}
	return filepath.Join(h, "slots")
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
