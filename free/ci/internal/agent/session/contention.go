package session

// Purpose: compose the node contention layer (internal/agent/contention, P7-NODE-13) into the session
//          (D-1226): lease admission through Gate.Acquire, the sleep check on every heartbeat tick, and
//          the policy a pull coordinator delivers in welcome.
// Inputs:  Config.Contention; each lease; the welcome.
// Outputs: a held slot per running lease; a refusal (log line plus a heartbeat naming the lease
//          "refused", with its node.* reason when the coordinator reads them); cancelled attempts after a
//          sleep; the delivered policy handed to Contention.Policy.
// Constraints: without Config.Contention the session takes plain slots, as before. A full table is still
//          E654. The reason is sent only after a welcome carrying the contention block (protocol
//          contention.go), so an older coordinator never sees the field. A lease that takes over an
//          attempt already running here (a re-lease after sleep) stops the older run first.

import (
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/agent/contention"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Contention wires the node's contention layer into a session.
type Contention struct {
	// Gate admits every lease (battery, interactive use, reservation) and holds its host slot. Required.
	Gate *contention.Gate
	// Wake runs on every heartbeat tick and returns the attempts whose slots were freed after the machine
	// slept (contention.SleepWatcher plus Gate.OnWake); the session cancels them. Nil checks nothing.
	Wake func() []string
	// Policy receives the policy a coordinator delivered in welcome (carrier B). Nil drops it.
	Policy func(protocol.NodePolicy)
}

// held is the host slot of one running lease and how to give it back.
type held struct {
	slot    *slots.Slot
	release func() error
}

// leaseNeed is what the lease's job asked of its slot.
func leaseNeed(l protocol.Lease) contention.Need {
	var n contention.Need
	if r := l.Job.Requirements; r != nil {
		n.CPU, n.MemMB = float64(r.CPU), int64(r.MemMB)
	}
	return n
}

// acquire takes the host slot of l; ok is false when l was refused (the refusal has been sent).
func (s *Session) acquire(l protocol.Lease) (held, bool) {
	owner := slots.Self(s.cfg.Coordinator.ID, l.LeaseID)
	if s.cfg.Contention == nil || s.cfg.Contention.Gate == nil {
		slot, err := s.cfg.Slots.Acquire(owner)
		if err != nil {
			s.refuseSlot(l, err)
			return held{}, false
		}
		return held{slot: slot, release: func() error { return s.cfg.Slots.Release(slot) }}, true
	}
	lease, d, err := s.cfg.Contention.Gate.Acquire(l.AttemptID, owner, ciexec.Remote, leaseNeed(l), false)
	switch {
	case err != nil:
		s.refuseSlot(l, err)
		return held{}, false
	case lease == nil:
		reason := string(d.Reason)
		s.refuseWith(l, "refused: "+reason, reason)
		return held{}, false
	}
	if lease.Replaced {
		for _, al := range s.active {
			if al.lease.AttemptID == l.AttemptID && !al.done.Load() {
				al.cancel() // its slot is already freed; the new lease runs the attempt
			}
		}
	}
	return held{slot: lease.Slot, release: lease.Release}, true
}

// refuseSlot refuses l for a slot table error: E654 for a full table.
func (s *Session) refuseSlot(l protocol.Lease, err error) {
	var se *slots.Error
	if errors.As(err, &se) {
		s.refuse(l, se.Reason, slots.CodeFull)
		return
	}
	s.refuse(l, "slot_table_error", "")
}

// applyWelcome hands a delivered policy to the node.
func (s *Session) applyWelcome() {
	c := s.welcome.Contention
	if c == nil || c.Policy == nil || s.cfg.Contention == nil || s.cfg.Contention.Policy == nil {
		return
	}
	s.cfg.Contention.Policy(*c.Policy)
}

// reasonRead reports whether the coordinator reads LeaseState.Reason.
func (s *Session) reasonRead() bool { return s.welcome.Contention != nil }

// wake cancels the attempts the contention layer freed after a sleep.
func (s *Session) wake() {
	if s.cfg.Contention == nil || s.cfg.Contention.Wake == nil {
		return
	}
	freed := s.cfg.Contention.Wake()
	if len(freed) == 0 {
		return
	}
	gone := map[string]bool{}
	for _, a := range freed {
		gone[a] = true
	}
	for _, al := range s.active {
		if gone[al.lease.AttemptID] {
			al.cancel()
		}
	}
}

// compatModes is what hello lists: the facts' modes, plus the contention marker when the layer is wired.
func (s *Session) compatModes() []protocol.CompatMode {
	modes := s.cfg.Facts.CompatModes
	if s.cfg.Contention == nil {
		return modes
	}
	return append(append([]protocol.CompatMode(nil), modes...), protocol.CompatContention)
}
