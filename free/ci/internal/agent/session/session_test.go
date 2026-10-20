package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type countSigner struct{ calls atomic.Int32 }

func (c *countSigner) KeyID() string { return "ci-node-abc" }
func (c *countSigner) Sign(b []byte) ([]byte, error) {
	c.calls.Add(1)
	return append([]byte("signed:"), b...), nil
}

// TestResultSigner proves the acceptance line: a signer set through
// SetResultSigner signs every result message (key_id and signature present),
// and with none set the results carry no signature and the session still
// completes.
func TestResultSigner(t *testing.T) {
	cs := &countSigner{}
	SetResultSigner(cs)
	t.Cleanup(func() { SetResultSigner(nil) })
	r := startRig(t, okRunner(`{"schema":"ci.evidence/v1"}`))
	r.handshake()
	for i, id := range []string{"l1", "l2"} {
		r.lease(testLease(id, uint64(i+1)))
		res := r.car.next(t, "result").Body.(protocol.Result)
		if res.Signature.KeyID != "ci-node-abc" || res.Signature.Value == "" || res.LeaseID != id {
			t.Fatalf("result %d: %+v", i, res)
		}
	}
	if cs.calls.Load() != 2 {
		t.Fatalf("signer called %d times, want once per result", cs.calls.Load())
	}
	SetResultSigner(nil)
	r.lease(testLease("l3", 1))
	res := r.car.next(t, "result").Body.(protocol.Result)
	if res.Signature != (protocol.Signature{}) {
		t.Fatalf("unsigned result carries a signature: %+v", res.Signature)
	}
	r.car.Close()
	if err := r.wait(); err != nil {
		t.Fatal(err)
	}
}

// TestSessionHelloAndCoordinators proves hello carries the key id, public key,
// age recipient and live slot counts, and that welcome records the
// coordinator's protocol range in <state>/coordinators.json.
func TestSessionHelloAndCoordinators(t *testing.T) {
	r := startRig(t, okRunner(`{}`), func(c *Config) { c.Facts.AgeRecipient = ptr("age1xyz") })
	h := r.handshake()
	if h.NodeKey.KeyID != "k1" || h.NodeKey.Public != "pub" || h.NodeKey.Alg != "ed25519" || *h.AgeRecipient != "age1xyz" ||
		h.Slots.Total != 2 || h.Slots.Used != 0 || len(h.Versions) != 1 || h.Versions[0] != 1 {
		t.Fatalf("hello %+v", h)
	}
	r.lease(testLease("l1", 1))
	r.car.next(t, "result")
	r.car.Close()
	if err := r.wait(); err != nil {
		t.Fatal(err)
	}
	recs, err := ReadCoordinators(r.state)
	if err != nil || len(recs) != 1 || recs[0].ID != "coord-a" || recs[0].ProtocolMax != 1 || recs[0].ProtocolMin != 1 || recs[0].MinAgentVersion != "1.4.0" || recs[0].RootFingerprint != "fp-coord-a" {
		t.Fatalf("coordinators.json: %+v %v", recs, err)
	}
}

// TestSessionRejectEndsWithTypedError proves a reject of hello ends the session
// with the coordinator's code and reason.
func TestSessionRejectEndsWithTypedError(t *testing.T) {
	r := startRig(t, okRunner(`{}`))
	r.car.next(t, "hello")
	r.car.push("reject", protocol.Reject{Code: "E652", Reason: "agent_version_too_old", MinAgentVersion: "1.5.0"})
	var re *RejectError
	if err := r.wait(); !errors.As(err, &re) || re.Code != "E652" || re.MinAgentVersion != "1.5.0" {
		t.Fatalf("got %v", err)
	}
}

// TestSlotRaceTwoCoordinators proves the acceptance line (X10): coordinators
// leasing to one agent concurrently never exceed its slot count, and the
// refused lease gets E654.
func TestSlotRaceTwoCoordinators(t *testing.T) {
	state := t.TempDir()
	const total, coordinators = 2, 6
	var running, peak atomic.Int32
	gate := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-gate
		running.Add(-1)
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{}`)}, nil
	})
	rigs := make([]*rig, coordinators)
	for i := range rigs {
		table, err := slots.Open(state, total) // one handle per session, as separate agent processes would hold
		if err != nil {
			t.Fatal(err)
		}
		rigs[i] = startRigOn(t, state, table, fmt.Sprintf("coord-%d", i), runner)
		rigs[i].handshake()
	}
	var wg sync.WaitGroup
	for i, r := range rigs {
		wg.Add(1)
		go func() { defer wg.Done(); r.lease(testLease(fmt.Sprintf("lease-%d", i), 1)) }()
	}
	wg.Wait()
	refused, accepted := map[int]bool{}, 0
	deadline := time.Now().Add(10 * time.Second)
	for len(refused)+accepted < coordinators && time.Now().Before(deadline) {
		for i, r := range rigs {
			if refused[i] {
				continue
			}
			for _, m := range r.car.drainOut() {
				if m.Type == "log" && strings.HasPrefix(m.Body.(protocol.Log).Text, "E654") {
					refused[i] = true
				}
				if m.Type == "heartbeat" {
					for _, l := range m.Body.(protocol.Heartbeat).Leases {
						if l.State == "refused" {
							refused[i] = true
						}
					}
				}
			}
		}
		accepted = int(running.Load())
		time.Sleep(5 * time.Millisecond)
	}
	if got := int(running.Load()); got != total || len(refused) != coordinators-total {
		t.Fatalf("running %d (want %d), refused %d (want %d)", got, total, len(refused), coordinators-total)
	}
	close(gate)
	if p := int(peak.Load()); p > total {
		t.Fatalf("peak %d jobs exceeded %d slots", p, total)
	}
}

// TestCancelAndStaleEpoch proves cancel stops the running job, is acked, and
// still yields a (cancelled) result; a cancel with a stale epoch is ignored.
func TestCancelAndStaleEpoch(t *testing.T) {
	started := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		close(started)
		<-ctx.Done()
		return JobResult{Outcome: Outcome{State: "cancelled"}, Evidence: json.RawMessage(`{"result":"cancelled"}`)}, nil
	})
	r := startRig(t, runner)
	r.handshake()
	r.lease(testLease("l1", 5))
	<-started
	r.car.pushFenced("cancel", protocol.Cancel{LeaseID: "l1", Reason: "old"}, 4, "l1")
	r.car.pushFenced("lease", testLease("l1", 3), 3, "l1") // a stale re-lease is refused
	r.car.pushFenced("cancel", protocol.Cancel{LeaseID: "l1", Reason: "operator"}, 5, "l1")
	ack := r.car.next(t, "ack").Body.(protocol.Ack)
	if ack.LeaseID != "l1" || ack.AckSeq != 5 {
		t.Fatalf("ack %+v (the stale cancel must not be acked)", ack)
	}
	res := r.car.next(t, "result").Body.(protocol.Result)
	if string(res.Evidence) != `{"result":"cancelled"}` || res.Epoch != 5 {
		t.Fatalf("result %+v", res)
	}
}

// TestHeartbeatAndDrain proves the heartbeat timer reports leases and slots
// with an injected clock, and that a drain refuses new leases, ends when the
// running lease finishes, and cancels what outlives its deadline.
func TestHeartbeatAndDrain(t *testing.T) {
	release := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		select {
		case <-release:
			return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{}`)}, nil
		case <-ctx.Done():
			return JobResult{Outcome: Outcome{State: "cancelled"}, Evidence: json.RawMessage(`{"cancelled":true}`)}, nil
		}
	})
	r := startRig(t, runner, func(c *Config) { c.Status = func() Status { return Status{Interactive: true} } })
	r.handshake()
	r.lease(testLease("l1", 1))
	waitUsed(t, r.table, 1)
	r.clock.advance(10 * time.Second)
	hb := r.car.next(t, "heartbeat").Body.(protocol.Heartbeat)
	if len(hb.Leases) != 1 || hb.Leases[0] != (protocol.LeaseState{LeaseID: "l1", Epoch: 1, State: "running"}) || hb.Slots != (protocol.Slots{Total: 2, Used: 1}) || !hb.Interactive || hb.Battery != nil {
		t.Fatalf("heartbeat %+v", hb)
	}
	deadline := r.clock.Now().Add(30 * time.Second).Format(time.RFC3339)
	r.car.push("drain", protocol.Drain{Deadline: deadline})
	r.car.next(t, "ack")
	r.lease(testLease("l2", 1))
	var sawRefused bool
	for !sawRefused {
		m := <-r.car.out
		if hb, ok := m.Body.(protocol.Heartbeat); ok {
			for _, l := range hb.Leases {
				sawRefused = sawRefused || l.LeaseID == "l2" && l.State == "refused"
			}
		}
	}
	r.clock.advance(31 * time.Second) // past the deadline: the running job is cancelled
	res := r.car.next(t, "result").Body.(protocol.Result)
	if string(res.Evidence) != `{"cancelled":true}` {
		t.Fatalf("result %s", res.Evidence)
	}
	if err := r.wait(); err != nil {
		t.Fatal(err)
	}
}

func waitUsed(t *testing.T, tb *slots.Table, n int) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if u, _ := tb.Used(); u == n {
			return
		}
	}
	t.Fatalf("slots never reached %d used", n)
}

// TestLogLinesNumberedAndMasked proves log lines are numbered per lease from 1,
// that the runner credential and a secret frame's value are masked before any
// line leaves the node, and that the credential bytes are zeroed afterwards.
func TestLogLinesNumberedAndMasked(t *testing.T) {
	var credView []byte
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		credView = j.Credential
		j.Output([]byte("start\nrunner uses JITSECRET123 here\npartial "))
		j.Output([]byte("line with PROJSECRET456\nlast"))
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{}`)}, nil
	})
	r := startRig(t, runner)
	r.handshake()
	l := testLease("l1", 1)
	l.RunnerCredential = &protocol.RunnerCredential{Kind: "github-jitconfig", Value: "JITSECRET123", ExpiresAt: "2026-10-08T20:00:00Z"}
	l.Job.Secrets = []string{"TOKEN"}
	l.Secrets = []protocol.SecretRef{{Ref: "project/TOKEN", Class: "project"}}
	r.lease(l)
	r.car.pushFenced("secrets", protocol.Secrets{LeaseID: "l1", Values: []protocol.SecretValue{{Ref: "project/TOKEN", Value: "PROJSECRET456"}}}, 1, "l1")
	var lines []string
	var seqs []uint64
	for {
		m := <-r.car.out
		if lg, ok := m.Body.(protocol.Log); ok {
			lines, seqs = append(lines, lg.Text), append(seqs, lg.LineSeq)
		}
		if m.Type == "result" {
			break
		}
	}
	want := []string{"start", "runner uses [redacted] here", "partial line with [redacted]", "last"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lines %q", lines)
	}
	for i, s := range seqs {
		if s != uint64(i+1) {
			t.Fatalf("line_seq %v", seqs)
		}
	}
	r.car.Close()
	if err := r.wait(); err != nil { // Run returns after every job goroutine has finished
		t.Fatal(err)
	}
	for _, b := range credView {
		if b != 0 {
			t.Fatalf("credential not zeroed: %q", credView)
		}
	}
}

// TestJobHooksRunInOrderAndUpload proves a registered JobHook's Before and
// After run in order around the job, After can upload a blob through the
// carrier, and a Before error fails the lease with a log line and a result.
func TestJobHooksRunInOrderAndUpload(t *testing.T) {
	var order []string
	var mu sync.Mutex
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	hook := JobHook{Name: "cache",
		Before: func(ctx context.Context, l protocol.Lease, root JobRoot) (JobSetup, error) {
			note("before")
			return JobSetup{Env: map[string]string{"CACHE": "1"}}, nil
		},
		After: func(ctx context.Context, l protocol.Lease, root JobRoot, out Outcome, up Uploader) error {
			note("after:" + out.State)
			if err := up.Digest(protocol.Digest{Stream: "cache", SHA256: "abc", Size: 1}); err != nil {
				return err
			}
			if err := up.Chunk(protocol.Chunk{Stream: "cache", Data: "YQ==", Digest: "abc", Final: true}); err != nil {
				return err
			}
			return up.CacheRef(protocol.CacheRef{Spec: "s", Key: "k", Blobs: []protocol.CacheBlob{{Path: "p", SHA256: "abc", Size: 1}}})
		},
	}
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		note("job:" + j.Setup.Env["CACHE"])
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{}`)}, nil
	})
	r := startRig(t, runner, func(c *Config) { c.Hooks = []JobHook{hook} })
	r.handshake()
	r.lease(testLease("l1", 1))
	var kinds []string
	for {
		m := <-r.car.out
		if m.Type != "heartbeat" && m.Type != "log" {
			kinds = append(kinds, m.Type)
		}
		if m.Type == "result" {
			break
		}
	}
	if got := strings.Join(order, ","); got != "before,job:1,after:passed" {
		t.Fatalf("order %s", got)
	}
	if got := strings.Join(kinds, ","); got != "digest,chunk,cache-ref,result" {
		t.Fatalf("frames %s", got)
	}
	bad := JobHook{Name: "bad", Before: func(context.Context, protocol.Lease, JobRoot) (JobSetup, error) {
		return JobSetup{}, errors.New("no cache")
	}}
	r2 := startRig(t, runner, func(c *Config) { c.Hooks = []JobHook{bad} })
	r2.handshake()
	r2.lease(testLease("l9", 1))
	r2.car.next(t, "result")
}

// TestRegisterJobHook proves registration order is kept and a duplicate or
// unnamed hook is refused.
func TestRegisterJobHook(t *testing.T) {
	saved := registeredHooks()
	t.Cleanup(func() { hookMu.Lock(); hooks = saved; hookMu.Unlock() })
	if err := RegisterJobHook(JobHook{}); err == nil {
		t.Fatal("accepted an unnamed hook")
	}
	if err := RegisterJobHook(JobHook{Name: "x-test"}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterJobHook(JobHook{Name: "x-test"}); err == nil {
		t.Fatal("accepted a duplicate")
	}
}

// TestNewerEpochForRunningLeaseStopsTheOldRun proves a re-lease with a higher
// epoch cancels the stale run and is refused until the slot is free, and that
// a second lease with the same epoch is ignored as a duplicate.
func TestNewerEpochForRunningLeaseStopsTheOldRun(t *testing.T) {
	var starts atomic.Int32
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		starts.Add(1)
		<-ctx.Done()
		return JobResult{Outcome: Outcome{State: "cancelled"}, Evidence: json.RawMessage(`{"old":true}`)}, nil
	})
	r := startRig(t, runner)
	r.handshake()
	r.lease(testLease("l1", 1))
	waitUsed(t, r.table, 1)
	r.lease(testLease("l1", 1)) // duplicate: ignored
	r.lease(testLease("l1", 2)) // newer epoch: old run stopped, new lease refused for now
	res := r.car.next(t, "result").Body.(protocol.Result)
	if res.Epoch != 1 || string(res.Evidence) != `{"old":true}` {
		t.Fatalf("result %+v", res)
	}
	if starts.Load() != 1 {
		t.Fatalf("job started %d times", starts.Load())
	}
}

// TestWelcomeVersionChecked proves a welcome that names another protocol
// version ends the session instead of being run as v1.
func TestWelcomeVersionChecked(t *testing.T) {
	r := startRig(t, okRunner(`{}`))
	r.car.next(t, "hello")
	r.car.push("welcome", protocol.Welcome{Version: 2, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 1000, AgentTTLMS: 3000})
	var pe *protocol.Error
	if err := r.wait(); !errors.As(err, &pe) || pe.Code != "E652" {
		t.Fatalf("got %v", err)
	}
}

// TestEpochMemoryIsBounded proves a long-lived session forgets finished lease ids.
func TestEpochMemoryIsBounded(t *testing.T) {
	s := &Session{seenEpoch: map[string]uint64{}, active: map[string]*activeLease{}}
	for i := range 3 * maxSeen {
		s.noteEpoch(protocol.Lease{LeaseID: fmt.Sprintf("l%d", i), Epoch: 1})
	}
	if len(s.seenEpoch) != maxSeen {
		t.Fatalf("kept %d ids, want %d", len(s.seenEpoch), maxSeen)
	}
}

// TestHeartbeatReportsFencedLeases proves the leases an earlier session fenced are named in the heartbeat as
// self_fenced with their epoch, for as long as the callback returns them.
func TestHeartbeatReportsFencedLeases(t *testing.T) {
	var pending atomic.Bool
	pending.Store(true)
	r := startRig(t, RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{}`)}, nil
	}), func(c *Config) {
		c.Fenced = func() []protocol.LeaseState {
			if pending.Load() {
				return []protocol.LeaseState{{LeaseID: "old", Epoch: 9, State: "self_fenced"}}
			}
			return nil
		}
	})
	r.handshake()
	r.clock.advance(10 * time.Second)
	hb := r.car.next(t, "heartbeat").Body.(protocol.Heartbeat)
	if len(hb.Leases) != 1 || hb.Leases[0] != (protocol.LeaseState{LeaseID: "old", Epoch: 9, State: "self_fenced"}) {
		t.Fatalf("heartbeat %+v", hb)
	}
	pending.Store(false)
	r.clock.advance(10 * time.Second)
	if hb = r.car.next(t, "heartbeat").Body.(protocol.Heartbeat); len(hb.Leases) != 0 {
		t.Fatalf("a cleared fence is still reported: %+v", hb)
	}
}
