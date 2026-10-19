package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// scriptCarrier delivers the queued frames, then blocks until closed.
type scriptCarrier struct {
	in     chan protocol.Message
	closed chan struct{}
	once   sync.Once
}

func newScriptCarrier() *scriptCarrier {
	return &scriptCarrier{in: make(chan protocol.Message, 16), closed: make(chan struct{})}
}
func (c *scriptCarrier) Send(protocol.Message) error { return nil }
func (c *scriptCarrier) Recv() (protocol.Message, error) {
	select {
	case m := <-c.in:
		return m, nil
	case <-c.closed:
		return protocol.Message{}, errors.New("closed")
	}
}
func (c *scriptCarrier) Close() error { c.once.Do(func() { close(c.closed) }); return nil }

func fenceRig(t *testing.T) (*fenceCarrier, *scriptCarrier, *atomic.Int64, string) {
	t.Helper()
	inner := newScriptCarrier()
	state := t.TempDir()
	f := newFenceCarrier(inner, state)
	var mono atomic.Int64
	f.newFence = func(ttl, hb time.Duration) *lease.AgentFence {
		return lease.NewAgentFence(func() time.Time { return time.Unix(0, 0).Add(time.Duration(mono.Load())) },
			func() time.Duration { return time.Duration(mono.Load()) }, ttl, hb)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, inner, &mono, state
}

func welcome() protocol.Message {
	return protocol.Message{V: 1, Type: "welcome", Body: &protocol.Welcome{Version: 1, HeartbeatMS: 40, AgentTTLMS: 400}}
}

// TestFenceEndsTheSessionWhenAcksStop proves a coordinator that goes silent past agent_ttl
// closes the carrier (the session then kills the jobs) and leaves a self_fenced marker.
func TestFenceEndsTheSessionWhenAcksStop(t *testing.T) {
	f, inner, mono, state := fenceRig(t)
	inner.in <- protocol.Message{V: 1, Type: "lease", Body: &protocol.Lease{LeaseID: "lease/9", Epoch: 1}}
	inner.in <- welcome()
	for range 2 {
		if _, err := f.Recv(); err != nil {
			t.Fatal(err)
		}
	}
	mono.Store(int64(10 * time.Second)) // far past the 400 ms agent_ttl, with no ack
	select {
	case <-inner.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the carrier stayed open after the coordinator went silent")
	}
	if _, err := os.Stat(filepath.Join(state, "self_fenced", "lease_9")); err != nil {
		t.Fatalf("no self_fenced marker: %v", err)
	}
}

// TestFenceHoldsWhileAcksArrive proves acknowledged heartbeats keep the session alive.
func TestFenceHoldsWhileAcksArrive(t *testing.T) {
	f, inner, mono, _ := fenceRig(t)
	inner.in <- welcome()
	if _, err := f.Recv(); err != nil {
		t.Fatal(err)
	}
	ack := uint64(1)
	for range 10 {
		mono.Add(int64(150 * time.Millisecond)) // under the 400 ms agent_ttl each time
		inner.in <- protocol.Message{V: 1, Type: "heartbeat", Ack: &ack, Body: &protocol.Heartbeat{}}
		if _, err := f.Recv(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond) // let the timer check run between acks
	}
	select {
	case <-inner.closed:
		t.Fatal("an acknowledged session was fenced")
	default:
	}
}

// TestSweepSkipsWhatALiveProcessHolds proves the orphan sweep removes dead roots and keeps held ones.
func TestSweepSkipsWhatALiveProcessHolds(t *testing.T) {
	state := t.TempDir()
	t.Setenv("NSELF_CI_AGENT_HOME", state)
	old := time.Now().Add(-48 * time.Hour)
	mk := func(rel string, hold bool) string {
		dir := filepath.Join(state, rel)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if hold {
			release, err := checkout.Hold(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)
		}
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	deadCo, liveCo := mk("checkout/co-dead-1", false), mk("checkout/co-live-1", true)
	deadJob, liveJob := mk("jobs/dead-1-abc", false), mk("jobs/live-1-abc", true)
	if err := sweepStaging(context.Background(), "serve"); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{deadCo, deadJob} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("%s survived the sweep", d)
		}
	}
	for _, d := range []string{liveCo, liveJob} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s was removed while held", d)
		}
	}
}
