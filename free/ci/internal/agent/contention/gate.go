package contention

// Purpose: the lease gate and the one-slot-per-attempt rule.
// Inputs:  Gate (slot table, probe, settings), the lease mode, the job's Need.
// Outputs: Decision; a held Lease whose Release frees the host slot.
// Constraints: the slot table is host-scoped, so a PERSONAL and a TEAM coordinator share one limit; the
//          cores and memory held are checked against the reservation share and the slot is taken under
//          one file lock, so concurrent coordinators never overshoot; Foreground is never gated (a
//          running foreground job takes no slot, so background work may fill the share beside it).

import (
	"errors"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Need is one job's request for its slot.
type Need = reservation.Need

// Decision is the verdict on one lease request.
type Decision struct {
	Allowed bool
	// Reason is the scheduler reason (node.on_battery, node.interactive, node.reservation) of a refusal.
	Reason model.PlacementReason
	// Drain is true when the refusal comes from on_battery=drain: the host should drain the node.
	Drain bool
	// Floor is true for the operator's own foreground run, which is always allowed.
	Floor   bool
	Share   reservation.Share
	Signals Signals
}

// Gate decides and enforces. The zero value is not usable; fill Table.
type Gate struct {
	// Dir is the agent state directory of Table; the reservation check and the slot take run under a
	// lock file in it, so every coordinator's gate on this node counts the same slots.
	Dir   string
	Table *slots.Table
	// Probe reads the host; nil uses exec.Probe (the one probe, CI-32).
	Probe func() exec.Capacity
	// Settings returns the node's policy; nil uses reservation.Defaults for an operator-owned node.
	Settings func() reservation.Settings
	// Signal returns an injected activity signal, or nil.
	Signal func() *Signals

	mu   sync.Mutex
	held map[string]*Lease
}

// Lease is one held slot of one attempt.
type Lease struct {
	Attempt string
	Slot    *slots.Slot
	owner   slots.Owner
	gate    *Gate
}

// Release frees the host slot and its hold; releasing twice changes nothing.
func (l *Lease) Release() error {
	g := l.gate
	return slots.WithLock(g.Dir, lockName, func() error {
		g.mu.Lock()
		if g.held[l.Attempt] == l {
			delete(g.held, l.Attempt)
		}
		g.mu.Unlock()
		return g.drop(l)
	})
}

// drop frees the slot of l and forgets its hold; the contention lock must be held.
func (g *Gate) drop(l *Lease) error {
	err := g.Table.Release(l.Slot)
	if werr := writeHolds(g.Dir, without(readHolds(g.Dir), l.owner, l.Attempt)); err == nil {
		err = werr
	}
	return err
}

func (g *Gate) probe() exec.Capacity {
	if g.Probe != nil {
		return g.Probe()
	}
	return exec.Probe()
}

func (g *Gate) settings(c exec.Capacity) reservation.Settings {
	if g.Settings != nil {
		return g.Settings()
	}
	return reservation.Defaults("", true, reservation.HasBattery(c))
}

// view takes one snapshot: the probe, the settings and the signals derived from them.
func (g *Gate) view() (exec.Capacity, reservation.Settings, Signals) {
	c := g.probe()
	var injected *Signals
	if g.Signal != nil {
		injected = g.Signal()
	}
	return c, g.settings(c), ReadSignals(c, injected)
}

// Admit judges a request against one snapshot without taking a slot.
func (g *Gate) Admit(mode exec.Mode, need Need, background bool) Decision {
	c, s, sig := g.view()
	return judge(c, s, sig, mode, need, background)
}

func judge(c exec.Capacity, s reservation.Settings, sig Signals, mode exec.Mode, need Need, background bool) Decision {
	d := Decision{Signals: sig, Share: reservation.Compute(c, s, need)}
	if mode == exec.Foreground {
		d.Allowed, d.Floor = true, true
		return d
	}
	switch {
	case sig.OnBattery && s.OnBattery != store.OnBatteryAllow:
		d.Reason, d.Drain = model.NodeOnBattery, s.OnBattery == store.OnBatteryDrain
	case s.Interactive == store.InteractiveProtect && sig.Interactive == Active && (background || mode == exec.Background):
		d.Reason = model.NodeInteractive
	case d.Share.Slots < 1:
		d.Reason = model.NodeReservation
	default:
		d.Allowed = true
	}
	return d
}

const lockName = "contention.lock"

// Acquire admits the request and takes a slot for attempt. An attempt that already holds a slot gives
// it up first, so one attempt never holds two leases (a re-lease after sleep replaces the old one).
// A full table returns the slots E654 error; a refusal returns Allowed false and a nil lease.
func (g *Gate) Acquire(attempt string, owner slots.Owner, mode exec.Mode, need Need, background bool) (*Lease, Decision, error) {
	c, s, sig := g.view()
	d := judge(c, s, sig, mode, need, background)
	if !d.Allowed || d.Floor {
		return nil, d, nil
	}
	if g.Dir == "" {
		return nil, d, errors.New("contention: Gate.Dir required")
	}
	var l *Lease
	var denied bool
	err := slots.WithLock(g.Dir, lockName, func() error {
		g.mu.Lock()
		old := g.held[attempt]
		g.mu.Unlock()
		holds := readHolds(g.Dir)
		if old != nil {
			holds = without(holds, old.owner, old.Attempt) // the old lease of this attempt is replaced, not added to
		}
		cpu := max(need.CPU, 1)
		if sh := d.Share; sh.Active {
			usedCPU, usedMem := totals(holds)
			if sh.CPUCapped && usedCPU+cpu > sh.CPU || sh.MemCapped && usedMem+need.MemMB > sh.MemMB {
				denied = true
				return nil
			}
		}
		if old != nil {
			if err := g.drop(old); err != nil {
				return err
			}
		}
		slot, err := g.Table.Acquire(owner)
		if err != nil {
			return err
		}
		l = &Lease{Attempt: attempt, Slot: slot, owner: owner, gate: g}
		if err := writeHolds(g.Dir, append(without(readHolds(g.Dir), owner, attempt), hold{Attempt: attempt, Owner: owner, CPU: cpu, MemMB: need.MemMB})); err != nil {
			_ = g.Table.Release(slot)
			l = nil
			return err
		}
		g.mu.Lock()
		if g.held == nil {
			g.held = map[string]*Lease{}
		}
		g.held[attempt] = l
		g.mu.Unlock()
		return nil
	})
	switch {
	case err != nil:
		return nil, Decision{Share: d.Share, Signals: d.Signals}, err
	case denied:
		return nil, Decision{Reason: model.NodeReservation, Share: d.Share, Signals: d.Signals}, nil
	}
	return l, d, nil
}

// OnWake runs after the machine slept. An intermittent node's leases are presumed expired at the
// coordinator, so every held slot is released (under the contention lock, so no Acquire slips between)
// and the attempts are returned for the caller to fence and kill: a job whose slot was freed is still a
// process until the caller stops it. A persistent node keeps its leases. The first release error is
// returned together with the attempts that were released.
func (g *Gate) OnWake() ([]string, error) {
	c := g.probe()
	if g.settings(c).Persistence != store.PersistenceInterm {
		return nil, nil
	}
	var out []string
	err := slots.WithLock(g.Dir, lockName, func() error {
		g.mu.Lock()
		ls := make([]*Lease, 0, len(g.held))
		for _, l := range g.held {
			ls = append(ls, l)
		}
		g.held = nil
		g.mu.Unlock()
		var first error
		for _, l := range ls {
			if e := g.drop(l); e != nil && first == nil {
				first = e
			}
			out = append(out, l.Attempt)
		}
		return first
	})
	return out, err
}

// Held is the number of leases this gate holds.
func (g *Gate) Held() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.held)
}
