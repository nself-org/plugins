package tart

// Purpose: count the macOS VM slots (Apple allows two at once) so they are never oversubscribed.
// Inputs:  the VMs `tart list` shows running, split into this profile family's clones (nself-ci-*) and others.
// Outputs: a ledger of attempts that hold a slot in this process, shared by the vm and ephemeral-vm profiles.
// Constraints: a reservation counts as busy from Wrap to Release, so a clone that is booting and not yet listed
//          is never free; clones of another coordinator process show in the listing and count as running.
//          A hold (the burst provider granting a slot before its job is wrapped) is a reservation made early:
//          it counts as busy at once, the Wrap of the same attempt takes it over instead of reserving a second
//          slot, and Teardown of an attempt that never reached Wrap gives it back.

import "sync"

// Slots is the in-process ledger.
type Slots struct {
	mu     sync.Mutex
	inUse  int             // attempts that hold a slot in this process
	booted int             // of those, VMs that were started (they show in the listing once Tart reports them running)
	held   map[string]bool // attempts that hold a slot before their Wrap (counted in inUse)
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
	// this process's reservations are all busy; listed clones beyond the ones it booted belong to other processes
	others := own - s.booted
	if others < 0 {
		others = 0
	}
	busy := s.inUse + others
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

// hold takes a slot for the attempt when one is free, ahead of its Wrap; holding twice takes one slot.
func (s *Slots) hold(id string, foreign, own int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[id] {
		return true
	}
	if s.freeLocked(foreign, own) == 0 {
		return false
	}
	if s.held == nil {
		s.held = map[string]bool{}
	}
	s.held[id] = true
	s.inUse++
	return true
}

// holds reports whether the attempt holds a slot ahead of its Wrap.
func (s *Slots) holds(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[id]
}

// takeOver turns the hold of the attempt into its reservation (no second slot is taken); false when it holds none.
func (s *Slots) takeOver(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.held[id] {
		return false
	}
	delete(s.held, id)
	return true
}

// unhold gives back the hold of an attempt that never reached Wrap; it does nothing for any other attempt.
func (s *Slots) unhold(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[id] {
		delete(s.held, id)
		if s.inUse > 0 {
			s.inUse--
		}
	}
}

// markBooted records that a reserved attempt started its VM.
func (s *Slots) markBooted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.booted < s.inUse {
		s.booted++
	}
}

// release gives a slot back; booted says whether the attempt had started its VM.
func (s *Slots) release(booted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inUse > 0 {
		s.inUse--
	}
	if booted && s.booted > 0 {
		s.booted--
	}
}
