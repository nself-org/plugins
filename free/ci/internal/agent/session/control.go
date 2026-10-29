package session

// Purpose: what the session does with each frame after the handshake, and the
// heartbeat and shutdown that go with it.
// Constraints: runs on the Run goroutine only (the lease table is not locked).

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func (s *Session) onMessage(m protocol.Message) (done bool, err error) {
	switch b := m.Body.(type) {
	case *protocol.Lease:
		s.onLease(m, *b)
	case *protocol.Secrets:
		s.onSecrets(m, *b)
	case *protocol.Cancel:
		return false, s.onCancel(m, *b)
	case *protocol.Drain:
		return s.onDrain(m, *b)
	case *protocol.Offset:
		s.onOffset(m, *b)
	case *protocol.Upgrade:
		s.onUpgrade(*b)
	case *protocol.Digest:
		s.onInbound(m, transport.BlobFrame{Digest: b})
	case *protocol.Chunk:
		s.onInbound(m, transport.BlobFrame{Chunk: b})
	}
	// heartbeat, ack, resume: the session ignores them.
	return false, nil
}

// onUpgrade answers an upgrade message with an upgrade-result: done after the swap (the service manager restarts the agent, which
// says hello again), refused with the E666 reason key, or nothing when the upgrade failed for another reason (the coordinator's
// hello check decides). Without a Config.Upgrade the message is ignored. The download runs off the Run goroutine so heartbeats go
// on; one upgrade runs at a time and a second message while it runs is ignored (the engine upgrades a node once at a time).
func (s *Session) onUpgrade(u protocol.Upgrade) {
	if s.cfg.Upgrade == nil || !s.upgrading.CompareAndSwap(false, true) {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.upgrading.Store(false)
		reason, err := s.cfg.Upgrade(s.runCtx, u)
		res := protocol.UpgradeResult{Version: u.Version, Status: protocol.UpgradeDone}
		switch {
		case reason != "":
			res.Status, res.Reason = protocol.UpgradeRefused, reason
		case err != nil:
			return
		}
		_ = s.send("upgrade-result", res, nil)
	}()
}

func (s *Session) onCancel(m protocol.Message, c protocol.Cancel) error {
	if l, ok := s.active[c.LeaseID]; ok {
		if m.Epoch == nil || *m.Epoch != l.lease.Epoch {
			return nil // not this run's epoch: a stale or future fence is ignored
		}
		l.cancel()
	}
	return s.send("ack", protocol.Ack{AckSeq: m.Seq, LeaseID: c.LeaseID}, nil)
}

func (s *Session) onDrain(m protocol.Message, d protocol.Drain) (bool, error) {
	s.draining = true
	if t, err := time.Parse(time.RFC3339, d.Deadline); err == nil {
		s.drainBy = t
	}
	if err := s.send("ack", protocol.Ack{AckSeq: m.Seq}, nil); err != nil {
		return false, err
	}
	return len(s.active) == 0, nil
}

// beat sends a heartbeat; extra names leases to report beyond the active ones
// (a refused lease). It also enforces the drain deadline.
func (s *Session) beat(extra []protocol.LeaseState) {
	if s.draining && !s.drainBy.IsZero() && !s.cfg.Clock.Now().Before(s.drainBy) {
		for _, l := range s.active {
			l.cancel()
		}
	}
	states := append([]protocol.LeaseState{}, extra...)
	for _, l := range s.active {
		states = append(states, protocol.LeaseState{LeaseID: l.lease.LeaseID, Epoch: l.lease.Epoch, State: l.state()})
	}
	used, err := s.cfg.Slots.Used()
	if err != nil {
		used = len(s.active)
	}
	var st Status
	if s.cfg.Status != nil {
		st = s.cfg.Status()
	}
	_ = s.send("heartbeat", protocol.Heartbeat{
		Leases: states, Slots: protocol.Slots{Total: s.cfg.Slots.Total(), Used: used},
		Battery: st.OnBattery, Interactive: st.Interactive,
	}, nil)
}

// shutdownGrace bounds how long shutdown waits for results to be written to a
// peer that has stopped reading.
var shutdownGrace = 20 * time.Second

// shutdown cancels every job and waits for its result to go out. When the
// peer is not reading (a blocked write), the carrier is closed after the grace
// period so no job goroutine outlives Run.
func (s *Session) shutdown() {
	close(s.closing)
	for _, l := range s.active {
		l.cancel()
	}
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		_ = s.cfg.Carrier.Close()
		<-done
	}
}
