package main

// Purpose: the agent half of the lease fence (Epic D6, P7-NODE-09) on the stdio carrier: when
// the coordinator stops acknowledging heartbeats for agent_ttl, the agent ends the session,
// which kills every job's process group, and records the leases as self_fenced.
// Inputs:  the carrier's frames: welcome (heartbeat_ms, agent_ttl_ms), lease, heartbeat acks.
// Outputs: a closed carrier; <state>/self_fenced/<lease> marker files; a line on stderr.
// Constraints: the fence is armed by welcome and checked on a timer, not only when a frame
// arrives, because a frozen coordinator sends nothing; closing the carrier is the same path
// as session loss, which the session already turns into a process-group kill.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

var fenceName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

type fenceCarrier struct {
	session.Carrier
	state string
	// newFence builds the fence; tests supply fake clocks.
	newFence func(coordinatorTTL, heartbeat time.Duration) *lease.AgentFence

	mu     sync.Mutex
	fence  *lease.AgentFence
	leases map[string]bool
	stop   chan struct{}
	once   sync.Once
}

func newFenceCarrier(inner session.Carrier, state string) *fenceCarrier {
	return &fenceCarrier{Carrier: inner, state: state, leases: map[string]bool{}, stop: make(chan struct{}),
		newFence: func(ttl, hb time.Duration) *lease.AgentFence { return lease.NewAgentFence(nil, nil, ttl, hb) }}
}

// Recv feeds the fence: welcome arms it, a heartbeat that acknowledges ours renews it.
func (c *fenceCarrier) Recv() (protocol.Message, error) {
	m, err := c.Carrier.Recv()
	if err != nil {
		return m, err
	}
	switch b := m.Body.(type) {
	case *protocol.Welcome:
		c.arm(*b)
	case *protocol.Lease:
		c.mu.Lock()
		c.leases[b.LeaseID] = true
		c.mu.Unlock()
	case *protocol.Heartbeat:
		c.mu.Lock()
		f := c.fence
		c.mu.Unlock()
		if f != nil && m.Ack != nil {
			f.Ack()
		}
	}
	return m, nil
}

// arm starts the fence. welcome carries agent_ttl, which is already coordinator_ttl - 2 x
// heartbeat, so the fence is built from the coordinator TTL that gives exactly that.
func (c *fenceCarrier) arm(w protocol.Welcome) {
	hb, ttl := time.Duration(w.HeartbeatMS)*time.Millisecond, time.Duration(w.AgentTTLMS)*time.Millisecond
	if hb <= 0 || ttl <= 0 {
		return
	}
	f := c.newFence(ttl+2*hb, hb)
	f.OnFence = c.onFence
	c.mu.Lock()
	armed := c.fence != nil
	if !armed {
		c.fence = f
	}
	c.mu.Unlock()
	if !armed {
		go c.watch(f, max(hb/2, 20*time.Millisecond))
	}
}

func (c *fenceCarrier) watch(f *lease.AgentFence, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			f.Check()
		}
	}
}

func (c *fenceCarrier) onFence() {
	c.mu.Lock()
	ids := make([]string, 0, len(c.leases))
	for id := range c.leases {
		ids = append(ids, id)
	}
	c.mu.Unlock()
	dir := filepath.Join(c.state, "self_fenced")
	if err := os.MkdirAll(dir, 0o700); err == nil {
		for _, id := range ids {
			_ = os.WriteFile(filepath.Join(dir, fenceName.ReplaceAllString(id, "_")), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
		}
	}
	fmt.Fprintln(os.Stderr, "fence: the coordinator stopped acknowledging heartbeats; ending the session and killing its jobs")
	_ = c.Carrier.Close()
}

// Close stops the fence timer and closes the carrier.
func (c *fenceCarrier) Close() error {
	c.once.Do(func() { close(c.stop) })
	return c.Carrier.Close()
}
