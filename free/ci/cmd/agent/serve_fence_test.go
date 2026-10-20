package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
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
func TestSweepSkipsWhatALiveProcessHolds(t *testing.T) { checkSweep(t) }

// checkSweep runs the dead-and-held-roots sweep case on the current host.
func checkSweep(t *testing.T) {
	t.Helper()
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

// TestServeStdioFencesItselfWhenTheCoordinatorGoesSilent proves the real serve path ends and the
// job's process group dies when no heartbeat is ever acknowledged (real clocks, short TTL).
func TestServeStdioFencesItselfWhenTheCoordinatorGoesSilent(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	p := startServe(t, "--coordinator", "peer-1")
	p.next("hello")
	p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 50, AgentTTLMS: 400}, 0, "")
	l := protocol.Lease{LeaseID: "fence1", AttemptID: "a-fence1", Epoch: 1, Token: "opaque-token-7f3a", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static", Run: []string{"sh", "-c", "echo started; sleep 307"}}, Checkout: protocol.Checkout{Revision: rev40},
		Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
	p.send("lease", l, l.Epoch, l.LeaseID)
	select { // never acknowledge a heartbeat
	case <-p.ended:
	case <-time.After(20 * time.Second):
		t.Fatal("serve kept running after the coordinator stopped acknowledging")
	}
	deadline := time.Now().Add(5 * time.Second)
	for exec.Command("pgrep", "-f", "[s]leep 307").Run() == nil {
		if time.Now().After(deadline) {
			t.Fatal("the job's process group survived the fence")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestFenceCountsCheckoutChunksAsLiveness proves, through the real wrapper chain, that a slow
// checkout upload does not fence the agent when the heartbeat acks are stuck behind the chunks.
func TestFenceCountsCheckoutChunksAsLiveness(t *testing.T) {
	inner := newScriptCarrier()
	var mono atomic.Int64
	car := wrapStdio(inner, t.TempDir(), func(in session.Carrier, st string) *fenceCarrier {
		f := newFenceCarrier(in, st)
		f.newFence = func(ttl, hb time.Duration) *lease.AgentFence {
			return lease.NewAgentFence(func() time.Time { return time.Unix(0, 0).Add(time.Duration(mono.Load())) },
				func() time.Duration { return time.Duration(mono.Load()) }, ttl, hb)
		}
		return f
	})
	t.Cleanup(func() { _ = car.Close() })
	go func() { // the session's read loop: checkout chunks are consumed below it
		for {
			if _, err := car.Recv(); err != nil {
				return
			}
		}
	}()
	inner.in <- welcome()
	time.Sleep(50 * time.Millisecond)
	for range 10 {
		mono.Add(int64(150 * time.Millisecond)) // under the 400 ms agent_ttl between chunks
		inner.in <- protocol.Message{V: 1, Type: "chunk", Body: &protocol.Chunk{Stream: "checkout", Data: "AAAA"}}
		time.Sleep(40 * time.Millisecond) // let the timer check run between chunks
	}
	select {
	case <-inner.closed:
		t.Fatal("an agent receiving its checkout was fenced")
	default:
	}
}

// TestFencedLeasesAreReportedUntilAnAckClearsThem: the lease a fence left a marker for is reported as
// self_fenced with its epoch by the next session, and a later heartbeat ack removes the marker.
func TestFencedLeasesAreReportedUntilAnAckClearsThem(t *testing.T) {
	f, inner, mono, state := fenceRig(t)
	inner.in <- protocol.Message{V: 1, Type: "lease", Body: &protocol.Lease{LeaseID: "lease/9", Epoch: 7}}
	inner.in <- welcome()
	for range 2 {
		if _, err := f.Recv(); err != nil {
			t.Fatal(err)
		}
	}
	mono.Store(int64(time.Second)) // far past agent_ttl: the watcher fences
	waitFor(t, "the fence marker", 5*time.Second, func() bool {
		ents, _ := os.ReadDir(filepath.Join(state, "self_fenced"))
		return len(ents) == 1
	})

	next := newFenceCarrier(newScriptCarrier(), state) // the next session of the same agent
	t.Cleanup(func() { _ = next.Close() })
	got := next.fenced()
	if len(got) != 1 || got[0].LeaseID != "lease/9" || got[0].Epoch != 7 || got[0].State != "self_fenced" {
		t.Fatalf("fenced() = %+v, want lease/9 epoch 7 self_fenced", got)
	}
	if again := next.fenced(); len(again) != 1 {
		t.Fatalf("an unacknowledged report must repeat, got %+v", again)
	}
	ack := uint64(1)
	next.Carrier.(*scriptCarrier).in <- protocol.Message{V: 1, Type: "heartbeat", Ack: &ack, Body: &protocol.Heartbeat{}}
	if _, err := next.Recv(); err != nil {
		t.Fatal(err)
	}
	if left := next.fenced(); len(left) != 0 {
		t.Fatalf("the acknowledged report was not cleared: %+v", left)
	}
}

// A marker that is not JSON, or has no lease, is dropped instead of being reported forever.
func TestFencedIgnoresAndDeletesBrokenMarkers(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(state, "self_fenced")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"old": "2026-01-01T00:00:00Z\n", "empty": `{"lease_id":""}`, "stale": `{"lease_id":"l","epoch":1,"at":"2001-01-01T00:00:00Z"}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := newFenceCarrier(newScriptCarrier(), state)
	t.Cleanup(func() { _ = f.Close() })
	if got := f.fenced(); len(got) != 0 {
		t.Fatalf("reported %+v", got)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("%d broken markers stayed", len(ents))
	}
}
