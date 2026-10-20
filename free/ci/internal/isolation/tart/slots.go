package tart

// Purpose: count the macOS VM slots (Apple allows two at once) so they are never oversubscribed.
// Inputs:  the VMs `tart list` shows running, split into this profile family's clones (nself-ci-*) and others.
// Outputs: a ledger of attempts that hold a slot in this process, shared by the vm and ephemeral-vm profiles.
// Constraints: a reservation counts as busy from Wrap to Release, so a clone that is booting and not yet listed
//          is never free; clones of another coordinator process show in the listing and count as running.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Slots is the in-process ledger.
type Slots struct {
	mu     sync.Mutex
	inUse  int             // attempts that hold a slot in this process
	booted int             // of those, VMs that were started (they show in the listing once Tart reports them running)
	held   map[string]bool // clone names whose marker this process holds or is about to claim
}

// NewSlots returns an empty ledger; the run hook shares one between both profiles.
func NewSlots() *Slots { return &Slots{} }

// hold records that this process owns the marker of name, so a peer count never takes it for another process's.
func (s *Slots) hold(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held == nil {
		s.held = map[string]bool{}
	}
	s.held[name] = true
}

func (s *Slots) unhold(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.held, name)
}

func (s *Slots) isHeld(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[name]
}

// bootingElsewhere counts the live attempts of other coordinator processes whose clone Tart does not list as
// running yet: their marker is locked, so the lock proves a booting VM that vmCounts cannot see (D-1355). Dead
// owners (unlocked markers) are the sweep's business and never count.
func (p *Profile) bootingElsewhere(dir string, vms []vmInfo) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	running := map[string]bool{}
	for _, v := range vms {
		if v.running() {
			running[v.Name] = true
		}
	}
	n := 0
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || name == e.Name() || !cloneName.MatchString(name) || running[name] || p.slots.isHeld(name) {
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, e.Name()), os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if ok, err := tryLock(f); err == nil && !ok {
			n++
		}
		_ = f.Close()
	}
	return n
}

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
