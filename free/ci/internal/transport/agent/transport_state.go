package agent

// Purpose: the transport-session state of the pull carrier: renumbering the buffered frames for a new
//          transport session (with the reconciling heartbeat first), the job tokens the upstream stream carries,
//          and the resume position (confirmed seq).
// Inputs:  the carrier's frame buffer, token set and seq counters; leases the long-poll delivers.
// Outputs: the first frame of a new session, token generations, adopted resume positions.
// Constraints: the caller of the unexported helpers holds c.mu where noted; a token is registered with the
//          upstream stream before the session sees its lease.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// resetTransport renumbers the buffered frames for a new transport session; the caller holds c.mu. Old
// heartbeats and acks are dropped (stale), and the latest heartbeat goes first so the coordinator reconciles
// the leases this agent kept.
func (c *Carrier) resetTransport() {
	c.nextT, c.confirmed, c.written = 0, 0, 0
	keep := c.out[:0]
	c.outBytes = 0
	for _, f := range c.out {
		if f.m.Type == "heartbeat" || f.m.Type == "ack" {
			continue
		}
		f.tseq = 0
		keep = append(keep, f)
		c.outBytes += f.size
	}
	c.out = keep
	if beat := c.reconcileBeat(); beat != nil {
		b, _ := encode(*beat, 1)
		c.out = append([]*outFrame{{m: *beat, size: len(b)}}, c.out...)
		c.outBytes += len(b)
	}
}

// reconcileBeat is the first frame of a new transport session: the latest heartbeat the session sent plus an
// entry for every lease this carrier delivered that the heartbeat does not name (it started after it), or just
// the delivered leases when the session sent none yet. Every lease the agent kept is named, so the coordinator
// rebinds each token to the new session. The caller holds c.mu.
func (c *Carrier) reconcileBeat() *protocol.Message {
	var hb protocol.Heartbeat
	if c.lastBeat != nil {
		if b, ok := c.lastBeat.Body.(protocol.Heartbeat); ok {
			hb = b
			hb.Leases = append([]protocol.LeaseState(nil), b.Leases...)
		}
	}
	named := map[string]bool{}
	for _, l := range hb.Leases {
		named[l.LeaseID] = true
	}
	for id := range c.tokens {
		if !named[id] {
			hb.Leases = append(hb.Leases, protocol.LeaseState{LeaseID: id, Epoch: c.epochs[id], State: "running"})
		}
	}
	if hb.Leases == nil {
		if c.lastBeat == nil {
			return nil
		}
		hb.Leases = []protocol.LeaseState{}
	}
	return &protocol.Message{Type: "heartbeat", Body: hb}
}

// registerToken records the token of a lease and waits until the upstream stream carries it.
func (c *Carrier) registerToken(ctx context.Context, l protocol.Lease) error {
	if l.Token == "" {
		return nil
	}
	c.mu.Lock()
	c.epochs[l.LeaseID] = l.Epoch
	c.tokenAt[l.LeaseID] = time.Now()
	fresh := c.tokens[l.LeaseID] != l.Token
	if fresh {
		c.tokens[l.LeaseID] = l.Token
		c.gen++
	}
	want := c.gen
	c.mu.Unlock()
	c.signal(c.wake)
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		c.mu.Lock()
		ok, ch := c.liveGen >= want, c.up
		c.mu.Unlock()
		if ok {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			c.forgetToken(l.LeaseID, fresh)
			return ctx.Err()
		case <-timer.C:
			c.forgetToken(l.LeaseID, fresh)
			return errors.New("carrier: the upstream stream did not pick up the job token")
		}
	}
}

// tokenSnapshot returns the current token generation and a copy of the tokens.
func (c *Carrier) tokenSnapshot() (int, map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make(map[string]string, len(c.tokens))
	for k, v := range c.tokens {
		cp[k] = v
	}
	return c.gen, cp
}

func (c *Carrier) setLive(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen > c.liveGen {
		c.liveGen = gen
		close(c.up)
		c.up = make(chan struct{})
	}
}

// adopt takes the coordinator's last accepted seq as confirmed when it lies between what the agent knew and
// what it sent.
func (c *Carrier) adopt(last uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if last < c.confirmed || last > c.nextT {
		return false
	}
	c.trim(last)
	return true
}

func (c *Carrier) confirmedSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.confirmed
}

// tokenGrace is how long a token outlives its lease's last mention in a heartbeat before it can be swept: a
// heartbeat composed just before the lease started must not take the new lease's token with it.
const tokenGrace = 2 * time.Minute

// sweepTokens forgets the tokens of leases that a heartbeat no longer names, that have no frame left in the
// buffer and that were registered more than tokenGrace ago (a lease that was refused or never ran sends no
// result, so nothing else would drop its token and the header would grow). The caller holds c.mu.
func (c *Carrier) sweepTokens(beat protocol.Message, now time.Time) {
	named := map[string]bool{}
	if hb, ok := beat.Body.(protocol.Heartbeat); ok {
		for _, l := range hb.Leases {
			named[l.LeaseID] = true
		}
	}
	busy := map[string]bool{}
	for _, f := range c.out {
		busy[f.m.LeaseID] = true
	}
	for id, at := range c.tokenAt {
		if named[id] || busy[id] || now.Sub(at) < tokenGrace {
			continue
		}
		delete(c.tokens, id)
		delete(c.epochs, id)
		delete(c.tokenAt, id)
	}
}

// dropRefused forgets the token of a lease the session refused to start ("refused" in a heartbeat): it will
// never produce a result, and its token must not stay in the stream header or in the reconcile beat. The
// caller holds c.mu.
func (c *Carrier) dropRefused(beat protocol.Message) {
	hb, ok := beat.Body.(protocol.Heartbeat)
	if !ok {
		return
	}
	for _, l := range hb.Leases {
		if l.State == "refused" {
			delete(c.tokens, l.LeaseID)
			delete(c.epochs, l.LeaseID)
			delete(c.tokenAt, l.LeaseID)
		}
	}
}

// forgetToken drops a token whose lease was never handed to the session, so it is not reported as running
// after the next reconnect. A token that was already known (a duplicate delivery) stays.
func (c *Carrier) forgetToken(leaseID string, fresh bool) {
	if !fresh {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, leaseID)
	delete(c.epochs, leaseID)
	delete(c.tokenAt, leaseID)
}
