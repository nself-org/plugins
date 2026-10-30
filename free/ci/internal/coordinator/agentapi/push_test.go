package agentapi

// Purpose: tests of the restore push window and the fence on upstream offset frames (P7-CACHE-21).

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestPushWindowIsAtomic(t *testing.T) {
	s := &Server{nodes: map[string]*nodeState{}}
	ns := s.state("n")
	ns.sess = &session{ctlLive: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.Push(ctx, "n", protocol.Message{Type: "chunk"}) }()
	}
	wg.Wait()
	s.mu.Lock()
	n := len(ns.ctl)
	s.mu.Unlock()
	if n != pushWindow {
		t.Fatalf("queued %d, want exactly the window of %d", n, pushWindow)
	}
	if err := s.Push(context.Background(), "gone", protocol.Message{Type: "chunk"}); err == nil {
		t.Fatal("a node with no control stream accepted a push")
	}
}

type offsetSink struct{ n atomic.Int32 }

func (*offsetSink) Frame(context.Context, string, protocol.Message) ([]protocol.Message, error) {
	return nil, nil
}
func (o *offsetSink) Offset(context.Context, string, protocol.Message) error { o.n.Add(1); return nil }

// A stale-epoch offset passes the generic fence: it is answered with an E662 cancel and never reaches the handler.
func TestStaleEpochOffsetRefused(t *testing.T) {
	h := &offsetSink{}
	r := newRig(t, func(c *Config) { c.Blobs = h })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStream(0)
	off := func(e uint64) protocol.Message {
		return protocol.Message{Type: "offset", LeaseID: leaseID, Epoch: &e, Body: protocol.Offset{Stream: "cache", SHA256: "ab", Offset: 1}}
	}
	u.send(off(epoch + 1))
	m := u.read()
	c, ok := m.Body.(*protocol.Cancel)
	if !ok || c.LeaseID != leaseID || len(c.Reason) < 4 || c.Reason[:4] != "E662" {
		t.Fatalf("answer = %s %+v, want an E662 cancel", m.Type, m.Body)
	}
	if h.n.Load() != 0 || r.eng.StaleCount() == 0 {
		t.Fatalf("stale offset reached the handler (%d) or was not counted", h.n.Load())
	}
	u.send(off(epoch))
	u.send(heartbeat())
	u.read() // heartbeat ack: the current-epoch offset before it was processed
	if h.n.Load() != 1 {
		t.Fatalf("current-epoch offset reached the handler %d times, want 1", h.n.Load())
	}
}
