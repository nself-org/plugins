package tart

// Purpose: count the macOS VM slots (Apple allows two at once) so they are never oversubscribed.
// Inputs:  the VMs `tart list` shows running, split into this profile family's clones (nself-ci-*) and others.
// Outputs: a ledger of attempts that hold a slot in this process, shared by the vm and ephemeral-vm profiles.
// Constraints: a reservation counts as busy from Wrap to Release, so a clone that is booting and not yet listed
//          is never free; clones of another coordinator process show in the listing and count as running.

import "sync"

// Slots is the in-process ledger.
type Slots struct {
	mu    sync.Mutex
	inUse int
}

// NewSlots returns an empty ledger; the run hook shares one between both profiles.
func NewSlots() *Slots { return &Slots{} }

// vmCounts splits the running VMs into foreign ones and the nself-ci clones.
func vmCounts(vms []vmInfo) (foreign, own int) {
	for _, v := range vms {
		switch {
		case !v.running():
		case len(v.Name) >= len(ClonePrefix) && v.Name[:len(ClonePrefix)] == ClonePrefix:
			own++
		default:
			foreign++
		}
	}
	return foreign, own
}

func (s *Slots) freeLocked(foreign, own int) int {
	busy := own
	if s.inUse > busy {
		busy = s.inUse
	}
	if free := MaxSlots - foreign - busy; free > 0 {
		return free
	}
	return 0
}

// Free is how many slots remain.
func (s *Slots) Free(foreign, own int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.freeLocked(foreign, own)
}

// reserve takes a slot when one is free.
func (s *Slots) reserve(foreign, own int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.freeLocked(foreign, own) == 0 {
		return false
	}
	s.inUse++
	return true
}

func (s *Slots) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inUse > 0 {
		s.inUse--
	}
}
