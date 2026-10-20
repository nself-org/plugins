package agent

// Purpose: unit tests of the pull driver and its sink without a listener: token minting, the node queue, the
//          wait for the result, cancel and its retry, lease loss, and the routing and dedup of agent output.
// Inputs:  fakes for the minter, the lease view, the cancel path and the run's sink.
// Constraints: bounded waits; no real keys.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type fakeMinter struct {
	mu     sync.Mutex
	claims []pki.TokenClaims
	limits []time.Duration
	err    error
}

func (f *fakeMinter) Mint(cl pki.TokenClaims, jobTimeout time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.claims, f.limits = append(f.claims, cl), append(f.limits, jobTimeout)
	return "token-" + cl.JTI, nil
}

type fakeLeases struct {
	mu    sync.Mutex
	state map[string]model.JobState
	err   error
}

func (f *fakeLeases) LeaseGet(_ context.Context, id string) (store.RemoteLease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return store.RemoteLease{}, f.err
	}
	st, ok := f.state[id]
	if !ok {
		return store.RemoteLease{}, &store.Error{Code: "E662", Message: "stale"}
	}
	return store.RemoteLease{ID: id, State: st}, nil
}

func (f *fakeLeases) set(id string, st model.JobState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state[id] = st
}

type cancelRec struct {
	mu    sync.Mutex
	calls []string
	fail  bool
}

func (c *cancelRec) cancel(node, lease, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, node+"/"+lease+"/"+reason)
	if c.fail {
		return errors.New("no control stream")
	}
	return nil
}

func (c *cancelRec) n() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.calls) }

type unitRig struct {
	d  *Driver
	m  *fakeMinter
	l  *fakeLeases
	c  *cancelRec
	rs *runSink
}

func newUnit(t *testing.T) *unitRig {
	t.Helper()
	u := &unitRig{m: &fakeMinter{}, l: &fakeLeases{state: map[string]model.JobState{}}, c: &cancelRec{}, rs: &runSink{}}
	u.d = &Driver{Minter: u.m, Leases: u.l, Cancel: u.c.cancel, Poll: 10 * time.Millisecond, CancelGrace: 300 * time.Millisecond, LogDir: t.TempDir()}
	return u
}

func nodeCap(id string) model.Capability {
	var c model.Capability
	c.Identity.ID = id
	return c
}

func (u *unitRig) run(ctx context.Context, node, lease string) <-chan error {
	u.l.set(lease, "running")
	l := protoLease(lease, "att-"+lease, 7)
	ch := make(chan error, 1)
	go func() { ch <- u.d.Run(ctx, nodeCap(node), l, u.rs) }()
	return ch
}

func (u *unitRig) waitQueued(t *testing.T, node string) *protocol.Lease {
	t.Helper()
	var l *protocol.Lease
	eventually(t, 3*time.Second, "the lease to be queued", func() bool {
		l, _ = u.d.Next(context.Background(), node)
		return l != nil
	})
	return l
}

func TestKindIsAgentAndRegistered(t *testing.T) {
	if (&Driver{}).Kind() != "agent" || Kind != "agent" {
		t.Fatal("kind")
	}
}

func TestRunNeedsAMinterAndALeaseView(t *testing.T) {
	for _, d := range []*Driver{{}, {Minter: &fakeMinter{}}, {Leases: &fakeLeases{}}} {
		err := d.Run(context.Background(), nodeCap("n"), protoLease("l", "a", 1), &runSink{})
		if ReasonOf(err) != ReasonNotConfigure {
			t.Fatalf("err = %v", err)
		}
	}
	u := newUnit(t)
	u.m.err = errors.New("no key")
	if err := u.d.Run(context.Background(), nodeCap("n"), protoLease("l", "a", 1), u.rs); ReasonOf(err) != ReasonNotConfigure || strings.Contains(err.Error(), "token-") {
		t.Fatalf("mint failure = %v", err)
	}
}

func TestRunQueuesALeaseWithASignedTokenForThisNodeAttemptAndEpoch(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := u.run(ctx, "n-1", "l1")
	l := u.waitQueued(t, "n-1")
	cancel()
	<-done
	u.m.mu.Lock()
	defer u.m.mu.Unlock()
	cl := u.m.claims[0]
	if l.LeaseID != "l1" || l.Token != "token-"+cl.JTI || cl.RunnerID != "n-1" || cl.AttemptID != "att-l1" || cl.Epoch != 7 || cl.Revision != strings.Repeat("a", 40) || cl.JTI == "" || cl.JobID == "" {
		t.Fatalf("lease %+v claims %+v", l, cl)
	}
	if u.m.limits[0] != time.Minute {
		t.Fatalf("token limit %v, want the job timeout (1m)", u.m.limits[0])
	}
	if len(cl.JTI) != 32 {
		t.Fatalf("jti %q is not 128 random bits", cl.JTI)
	}
}

func TestJobTimeoutParsing(t *testing.T) {
	for in, want := range map[string]time.Duration{"": DefaultJobTimeout, "garbage": DefaultJobTimeout, "-5m": DefaultJobTimeout, "0s": DefaultJobTimeout, "90s": 90 * time.Second, "2h": 2 * time.Hour} {
		if got := jobTimeout(model.Job{Timeout: in}); got != want {
			t.Errorf("%q -> %v, want %v", in, got, want)
		}
	}
}

func TestEachNodeGetsItsOwnLeasesOldestFirst(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, p := range [][2]string{{"a", "l1"}, {"b", "l2"}, {"a", "l3"}} {
		u.run(ctx, p[0], p[1])
		time.Sleep(20 * time.Millisecond)
	}
	var got []string
	eventually(t, 3*time.Second, "all leases queued", func() bool {
		u.d.mu.Lock()
		defer u.d.mu.Unlock()
		return len(u.d.queues["a"]) == 2 && len(u.d.queues["b"]) == 1
	})
	for _, n := range []string{"a", "b", "a", "a", "b"} {
		l, err := u.d.Next(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		if l == nil {
			got = append(got, n+":-")
		} else {
			got = append(got, n+":"+l.LeaseID)
		}
	}
	if strings.Join(got, " ") != "a:l1 b:l2 a:l3 a:- b:-" {
		t.Fatalf("Next order = %v", got)
	}
}

func TestDuplicateLeaseAndFullQueueAreRefused(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	err := u.d.Run(ctx, nodeCap("n-1"), protoLease("l1", "att-l1", 7), u.rs)
	if ReasonOf(err) != ReasonDuplicate {
		t.Fatalf("duplicate = %v", err)
	}
	u.d.mu.Lock()
	for i := 0; i < maxQueued; i++ {
		u.d.queues["full"] = append(u.d.queues["full"], protocol.Lease{LeaseID: fmt.Sprint(i)})
	}
	u.d.mu.Unlock()
	u.l.set("lx", "running")
	err = u.d.Run(ctx, nodeCap("full"), protoLease("lx", "att-lx", 7), u.rs)
	if ReasonOf(err) != ReasonQueueFull {
		t.Fatalf("full queue = %v", err)
	}
	u.d.mu.Lock()
	_, routed := u.d.routes["lx"]
	u.d.mu.Unlock()
	if routed {
		t.Fatal("a refused lease left a route behind")
	}
}

func TestRunReturnsLostWhenTheLeaseIsGone(t *testing.T) {
	for name, set := range map[string]func(*unitRig){
		"attempt lost":      func(u *unitRig) { u.l.set("l1", "lost") },
		"attempt cancelled": func(u *unitRig) { u.l.set("l1", "cancelled") },
		"lease unknown":     func(u *unitRig) { u.l.mu.Lock(); delete(u.l.state, "l1"); u.l.mu.Unlock() },
	} {
		u := newUnit(t)
		done := u.run(context.Background(), "n-1", "l1")
		u.waitQueued(t, "n-1")
		set(u)
		select {
		case err := <-done:
			if ReasonOf(err) != ReasonLeaseLost {
				t.Fatalf("%s: Run = %v", name, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: Run did not notice", name)
		}
	}
}

func TestRunKeepsWaitingOnAStoreHiccupAndWhileFinalizing(t *testing.T) {
	u := newUnit(t)
	var notes []string
	var mu sync.Mutex
	u.d.Logf = func(f string, a ...any) { mu.Lock(); notes = append(notes, fmt.Sprintf(f, a...)); mu.Unlock() }
	done := u.run(context.Background(), "n-1", "l1")
	u.waitQueued(t, "n-1")
	u.l.mu.Lock()
	u.l.err = errors.New("database is locked")
	u.l.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	u.l.mu.Lock()
	u.l.err = nil
	u.l.mu.Unlock()
	u.l.set("l1", "finalizing")
	select {
	case err := <-done:
		t.Fatalf("Run returned early: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	mu.Lock()
	logged := len(notes) > 0
	mu.Unlock()
	if !logged {
		t.Fatal("a store failure was not logged")
	}
}

func TestResultFinishesRunSpendsTheTokenAndIsIdempotent(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	d := &Driver{Gate: r.gate(), Minter: r.tokens, Leases: r.st, Poll: 10 * time.Millisecond}
	rs := &runSink{eng: r.eng}
	done := make(chan error, 1)
	go func() { done <- d.Run(context.Background(), nodeCap("n-1"), protoLease(lease, att, ep), rs) }()
	var l *protocol.Lease
	eventually(t, 3*time.Second, "queued", func() bool { l, _ = d.Next(context.Background(), "n-1"); return l != nil })
	cl, _ := r.tokens.Verify(l.Token)
	res := protocol.Result{LeaseID: lease, Epoch: ep, Evidence: []byte(`{}`), Signature: protocol.Signature{KeyID: "k", Value: "v"}}
	if err := d.Sink().Result(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	if b, _, _ := r.st.TokenCheck(context.Background(), cl.JTI); b.JTI != "" && !b.Spent {
		t.Fatal("token not spent")
	}
	if err := d.Sink().Result(context.Background(), res); err != nil { // a resend after Run returned
		t.Fatalf("late result = %v", err)
	}
	if rs.nResults() != 1 {
		t.Fatalf("%d results reached the run", rs.nResults())
	}
	if !d.Sink().(interface{ CommitsResult() bool }).CommitsResult() {
		t.Fatal("the sink must say it commits results")
	}
}

func TestSinkRefusesAFrameForALeaseItNeverDelivered(t *testing.T) {
	u := newUnit(t)
	s := u.d.Sink()
	ctx := context.Background()
	for name, err := range map[string]error{
		"log":      s.Log(ctx, protocol.Log{LeaseID: "ghost", LineSeq: 1}),
		"result":   s.Result(ctx, protocol.Result{LeaseID: "ghost"}),
		"artifact": s.ArtifactRef(ctx, protocol.ArtifactRef{LeaseID: "ghost"}),
		"cache":    s.CacheRef(ctx, protocol.CacheRef{LeaseID: "ghost"}),
	} {
		if ReasonOf(err) != ReasonLeaseLost {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.Blob(ctx, transportHeader()); err == nil {
		t.Fatal("Blob must not be callable directly")
	}
}

func TestSinkForwardsReferencesAndDropsDuplicateLines(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	s := u.d.Sink()
	for _, seq := range []uint64{1, 2, 2, 1, 3} {
		if err := s.Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: seq, Stream: "stdout", Text: fmt.Sprint("line-", seq)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := u.rs.lines(); strings.Join(got, ",") != "line-1,line-2,line-3" {
		t.Fatalf("run saw %v", got)
	}
	if err := s.ArtifactRef(ctx, protocol.ArtifactRef{LeaseID: "l1", Path: "a", SHA256: "x", Size: 1}); err != nil || len(u.rs.refs) != 1 {
		t.Fatalf("artifact ref: %v %d", err, len(u.rs.refs))
	}
	if err := s.CacheRef(ctx, protocol.CacheRef{LeaseID: "l1", Spec: "s", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	// a failing run sink fails the frame, so the agent keeps and resends it
	u.rs.failLog = errors.New("run is gone")
	if err := s.Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: 4, Text: "x"}); err == nil {
		t.Fatal("a run sink failure was swallowed")
	}
}

func TestSinkWithoutALogDirForwardsEverything(t *testing.T) {
	u := newUnit(t)
	u.d.LogDir = ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	for _, seq := range []uint64{1, 1} {
		_ = u.d.Sink().Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: seq, Text: "x"})
	}
	if len(u.rs.lines()) != 2 {
		t.Fatal("without a durable log every frame is forwarded")
	}
}

func TestSinkLogOpenFailureIsAnError(t *testing.T) {
	u := newUnit(t)
	u.d.LogDir = "/dev/null/not-a-dir"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	if err := u.d.Sink().Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: 1, Text: "x"}); err == nil || !strings.Contains(err.Error(), "lease log") {
		t.Fatalf("err = %v", err)
	}
}

func TestLateFramesOfAFinishedLeaseAreAcceptedAndDropped(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	cancel()
	<-done
	s := u.d.Sink()
	if err := s.Log(context.Background(), protocol.Log{LeaseID: "l1", LineSeq: 9, Text: "late"}); err != nil {
		t.Fatalf("late log = %v", err)
	}
	if len(u.rs.lines()) != 0 {
		t.Fatal("a late line reached the run")
	}
}

func TestCancelAsksTheAgentAndWaitsForTheResult(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	cancel()
	eventually(t, 3*time.Second, "the cancel to be sent", func() bool { return u.c.n() == 1 })
	u.c.mu.Lock()
	if u.c.calls[0] != "n-1/l1/cancelled" {
		t.Fatalf("cancel = %v", u.c.calls)
	}
	u.c.mu.Unlock()
	if err := u.d.Sink().Result(context.Background(), protocol.Result{LeaseID: "l1", Epoch: 7}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run after a cancel with a result = %v", err)
	}
}

func TestCancelWithoutAResultTimesOut(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	cancel()
	select {
	case err := <-done:
		if ReasonOf(err) != ReasonCancelLate {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not give up")
	}
	u.d.mu.Lock()
	_, kept := u.d.pending["n-1"]["l1"]
	u.d.mu.Unlock()
	if kept {
		t.Fatal("a finished run left a pending cancel")
	}
}

func TestACancelThatFindsNoStreamIsRetriedWhenTheNodePolls(t *testing.T) {
	u := newUnit(t)
	u.c.fail = true
	u.d.CancelGrace = 3 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	cancel()
	eventually(t, 3*time.Second, "the first cancel attempt", func() bool { return u.c.n() >= 1 })
	before := u.c.n()
	if _, err := u.d.Next(context.Background(), "n-1"); err != nil { // the node is back and polls
		t.Fatal(err)
	}
	if u.c.n() != before+1 {
		t.Fatalf("the cancel was not retried on the poll (%d calls, had %d)", u.c.n(), before)
	}
	u.c.mu.Lock()
	u.c.fail = false
	u.c.mu.Unlock()
	_, _ = u.d.Next(context.Background(), "n-1")
	n := u.c.n()
	_, _ = u.d.Next(context.Background(), "n-1")
	if u.c.n() != n {
		t.Fatal("a delivered cancel was sent again")
	}
	_ = u.d.Sink().Result(context.Background(), protocol.Result{LeaseID: "l1", Epoch: 7})
	<-done
}

func TestCancelWithoutACancelPathStillWaitsForTheGrace(t *testing.T) {
	u := newUnit(t)
	u.d.Cancel = nil
	ctx, cancel := context.WithCancel(context.Background())
	done := u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	cancel()
	select {
	case err := <-done:
		if ReasonOf(err) != ReasonCancelLate {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no timeout")
	}
	if _, err := u.d.Next(context.Background(), "n-1"); err != nil {
		t.Fatal(err)
	}
}

func TestWireAndSetCancelReplaceTheSettings(t *testing.T) {
	d := &Driver{}
	m, l := &fakeMinter{}, &fakeLeases{}
	var noted []string
	d.Wire(Wiring{Minter: m, Leases: l, LogDir: "/x", Logf: func(f string, a ...any) { noted = append(noted, fmt.Sprintf(f, a...)) }})
	c := &cancelRec{}
	d.SetCancel(c.cancel)
	got := d.cfg()
	if got.minter != Minter(m) || got.leases != LeaseView(l) || got.logDir != "/x" || got.cancel == nil || got.logf == nil || got.gate != nil {
		t.Fatalf("cfg = %+v", got)
	}
	d.logf("hello %d", 1)
	if len(noted) != 1 || noted[0] != "hello 1" {
		t.Fatalf("logf = %v", noted)
	}
	(&Driver{}).logf("no logger set") // must not panic
}

func TestPruneRunsAtMostEveryTenMinutes(t *testing.T) {
	cb := &pruneCount{}
	g := &Gate{Bindings: cb}
	d := &Driver{}
	d.prune(context.Background(), g, time.Now())
	d.prune(context.Background(), g, time.Now())
	d.prune(context.Background(), nil, time.Now())
	if cb.n != 1 {
		t.Fatalf("pruned %d times, want 1", cb.n)
	}
	d.mu.Lock()
	d.lastPrune = time.Now().Add(-pruneEvery - time.Second)
	d.mu.Unlock()
	d.prune(context.Background(), g, time.Now())
	if cb.n != 2 {
		t.Fatalf("pruned %d times after the interval, want 2", cb.n)
	}
	cb.err = errors.New("locked")
	d.mu.Lock()
	d.lastPrune = time.Time{}
	d.mu.Unlock()
	var noted []string
	d.Logf = func(f string, a ...any) { noted = append(noted, f) }
	d.prune(context.Background(), g, time.Now())
	if len(noted) != 1 {
		t.Fatal("a failed prune was not logged")
	}
}

type pruneCount struct {
	Bindings
	n   int
	err error
}

func (p *pruneCount) TokenPrune(context.Context, int64) (int64, error) { p.n++; return 0, p.err }

func TestAdmitFailsClosedWithoutAGateAndSyncsLogsBeforeAHeartbeatAck(t *testing.T) {
	u := newUnit(t)
	if err := u.d.Admit(context.Background(), agentapi.Call{LeaseID: "l1", Type: "log"}); err == nil || DenialReason(err) != "" {
		t.Fatalf("without a gate Admit = %v, want a plain failure (not a refusal that cancels the lease)", err)
	}
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	d := &Driver{Gate: r.gate(), Minter: r.tokens, Leases: r.st, LogDir: t.TempDir(), Poll: 10 * time.Millisecond}
	rs := &runSink{eng: r.eng}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = d.Run(ctx, nodeCap("n-1"), protoLease(lease, att, ep), rs) }()
	var l *protocol.Lease
	eventually(t, 3*time.Second, "queued", func() bool { l, _ = d.Next(ctx, "n-1"); return l != nil })
	if err := d.Sink().Log(ctx, protocol.Log{LeaseID: lease, LineSeq: 1, Stream: "stdout", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	lw := d.routes[lease].lw
	d.mu.Unlock()
	lw.mu.Lock()
	dirty := lw.dirty
	lw.mu.Unlock()
	if !dirty {
		t.Fatal("a log line is on disk before any sync was asked for")
	}
	call := agentapi.Call{Node: "n-1", Session: "s", Serial: "aa", LeaseID: lease, Epoch: ep, Token: l.Token}
	call.Type = "log"
	if err := d.Admit(ctx, call); err != nil {
		t.Fatal(err)
	}
	lw.mu.Lock()
	still := lw.dirty
	lw.mu.Unlock()
	if !still {
		t.Fatal("a log call synced the logs")
	}
	call.Type = "heartbeat"
	if err := d.Admit(ctx, call); err != nil {
		t.Fatal(err)
	}
	lw.mu.Lock()
	clean := !lw.dirty
	lw.mu.Unlock()
	if !clean {
		t.Fatal("the logs were not put on stable storage before the heartbeat could be acknowledged")
	}
	call.Token = "forged"
	if err := d.Admit(ctx, call); DenialReason(err) != ReasonInvalid {
		t.Fatalf("a forged token = %v", err)
	}
}

func TestARunSinkThatFailsALineDoesNotLoseItOnResend(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	s := u.d.Sink()
	u.rs.failLog = errors.New("run busy")
	if err := s.Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: 1, Text: "one"}); err == nil {
		t.Fatal("the failure was swallowed")
	}
	u.rs.mu.Lock()
	u.rs.failLog = nil
	u.rs.mu.Unlock()
	for _, seq := range []uint64{1, 1, 2} { // the agent resends line 1, then sends line 2
		if err := s.Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: seq, Text: fmt.Sprint("line-", seq)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(u.rs.lines(), ","); got != "line-1,line-2" {
		t.Fatalf("run saw %q, want each line once", got)
	}
	if lw := u.d.routes["l1"].lw; lw.Last() != 2 {
		t.Fatalf("durable tail %d", lw.Last())
	}
}
