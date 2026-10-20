package agent

// Purpose: boundary and branch tests that pin small decisions of the driver, the gate and the carrier (what a
//          mutation run would otherwise leave alive): error text, defaults, cache limits, retry counts, bookkeeping.
// Inputs:  the rig, scripted servers, and fakes of the store seams.
// Constraints: no sleeps; bounded waits.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestErrorTextHasTheMessageOnlyWhenThereIsOne(t *testing.T) {
	if got := (&Error{Code: "E656", Reason: "x"}).Error(); got != "E656 x" {
		t.Fatalf("%q", got)
	}
	if got := (&Error{Code: "E656", Reason: "x", Msg: "why"}).Error(); got != "E656 x: why" {
		t.Fatalf("%q", got)
	}
}

func TestShortCutsADigestToTwelveCharacters(t *testing.T) {
	for in, want := range map[string]string{"": "", "abc": "abc", "123456789012": "123456789012", "1234567890123": "123456789012"} {
		if got := short(in); got != want {
			t.Errorf("short(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDriverDefaultsApplyOnlyWhenNothingIsSet(t *testing.T) {
	d := &Driver{}
	if d.grace() != 5*time.Second || d.poll() != time.Second {
		t.Fatalf("defaults %v %v", d.grace(), d.poll())
	}
	d = &Driver{CancelGrace: 7 * time.Millisecond, Poll: 3 * time.Millisecond}
	if d.grace() != 7*time.Millisecond || d.poll() != 3*time.Millisecond {
		t.Fatalf("configured %v %v", d.grace(), d.poll())
	}
}

func TestAdmitSyncsTheLogsOnAHeartbeatAndOnlyThen(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	logged := 0
	d := &Driver{Gate: r.gate(), Logf: func(string, ...any) { logged++ }}
	lw, err := OpenLog(LogFile(t.TempDir(), lease))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lw.Close() })
	d.routes = map[string]*route{lease: {leaseID: lease, lw: lw}}
	beatCall := call(lease, tok, ep)
	beatCall.Type = "heartbeat"
	if _, err = lw.Append(logOf(1, "a")); err != nil {
		t.Fatal(err)
	}
	if err = d.Admit(context.Background(), beatCall); err != nil || logged != 0 {
		t.Fatalf("Admit = %v, logged %d", err, logged)
	}
	lw.mu.Lock()
	dirty := lw.dirty
	lw.mu.Unlock()
	if dirty {
		t.Fatal("a heartbeat must put the log on stable storage")
	}
	if _, err = lw.Append(logOf(2, "b")); err != nil {
		t.Fatal(err)
	}
	lw.mu.Lock()
	_ = lw.f.Close() // the next sync fails
	lw.mu.Unlock()
	logCall := beatCall
	logCall.Type = "log"
	if err = d.Admit(context.Background(), logCall); err != nil {
		t.Fatalf("a log frame does not sync: %v", err)
	}
	if err = d.Admit(context.Background(), beatCall); err == nil || logged != 1 {
		t.Fatalf("a failing sync must fail the heartbeat and be logged: %v, logged %d", err, logged)
	}
}

type pruneAt struct {
	Bindings
	n int
}

func (p *pruneAt) TokenPrune(context.Context, int64) (int64, error) { p.n++; return 0, nil }

func TestPruneRunsExactlyAtTheInterval(t *testing.T) {
	pc := &pruneAt{}
	g := &Gate{Bindings: pc}
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	d := &Driver{lastPrune: t0}
	d.prune(context.Background(), g, t0.Add(pruneEvery-time.Nanosecond))
	if pc.n != 0 {
		t.Fatal("pruned a nanosecond early")
	}
	d.prune(context.Background(), g, t0.Add(pruneEvery))
	if pc.n != 1 {
		t.Fatal("did not prune at the interval")
	}
}

func TestGateCacheResetsOnlyAtItsLimit(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	fill := func(n int) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.known = map[use]bool{}
		for i := 0; i < n; i++ {
			g.known[use{"x" + strconv.Itoa(i), "s", "t"}] = true
		}
	}
	size := func() int { g.mu.Lock(); defer g.mu.Unlock(); return len(g.known) }
	fill(maxKnown - 1)
	if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil || size() != maxKnown {
		t.Fatalf("below the limit: err %v size %d", err, size())
	}
	fill(maxKnown)
	if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil || size() != 1 {
		t.Fatalf("at the limit: err %v size %d", err, size())
	}
}

func TestGateSpendResetsItsMemoryOnlyAtTheLimitAndForgetsOnlyThatToken(t *testing.T) {
	r := newRig(t)
	l1, a1, e1 := r.seedLease("1", "n-1")
	l2, a2, e2 := r.seedLease("2", "n-1")
	t1, t2 := r.mint("jti-1", "n-1", a1, e1), r.mint("jti-2", "n-1", a2, e2)
	g := r.gate()
	ctx := context.Background()
	if err := g.Check(ctx, call(l1, t1, e1)); err != nil {
		t.Fatal(err)
	}
	if err := g.Check(ctx, call(l2, t2, e2)); err != nil {
		t.Fatal(err)
	}
	if err := g.Spend(ctx, "jti-1"); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	if len(g.known) != 1 || !g.known[use{"jti-2", "aa01", "sess-1"}] {
		t.Fatalf("known after spending jti-1: %v", g.known)
	}
	g.spent = map[string]bool{}
	for i := 0; i < maxKnown-1; i++ {
		g.spent["x"+strconv.Itoa(i)] = true
	}
	g.mu.Unlock()
	if err := g.Spend(ctx, "jti-2"); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	if len(g.spent) != maxKnown {
		t.Fatalf("below the limit: %d", len(g.spent))
	}
	g.mu.Unlock()
	if err := g.Spend(ctx, "jti-3"); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.spent) != 1 || !g.spent["jti-3"] {
		t.Fatalf("at the limit: %d entries", len(g.spent))
	}
}

type scriptBindings struct {
	Bindings
	check func() (store.TokenBinding, bool, error)
}

func (s *scriptBindings) TokenRebind(context.Context, string, string, string, string, string) (bool, error) {
	return false, nil // another call moved the row first
}
func (s *scriptBindings) TokenCheck(context.Context, string) (store.TokenBinding, bool, error) {
	return s.check()
}

func TestGateAfterALostRebindRaceAdmitsOnlyTheWinnerSession(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	if err := r.gate().Check(context.Background(), call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("store down")
	for name, tc := range map[string]struct {
		b      store.TokenBinding
		found  bool
		err    error
		reason string // "" admits
	}{
		"winner is this session": {b: store.TokenBinding{Serial: "aa01", Session: "sess-2"}, found: true},
		"row vanished":           {found: false, reason: ReasonWrongSession},
		"other serial":           {b: store.TokenBinding{Serial: "zz", Session: "sess-2"}, found: true, reason: ReasonWrongSession},
		"other session":          {b: store.TokenBinding{Serial: "aa01", Session: "sess-9"}, found: true, reason: ReasonWrongSession},
		"winner is spent":        {b: store.TokenBinding{Serial: "aa01", Session: "sess-2", Spent: true}, found: true, reason: ReasonReplayed},
		"spent, other session":   {b: store.TokenBinding{Serial: "aa01", Session: "sess-1", Spent: true}, found: true, reason: ReasonReplayed},
		"store failure":          {err: boom},
	} {
		t.Run(name, func(t *testing.T) {
			sb := &scriptBindings{Bindings: r.st, check: func() (store.TokenBinding, bool, error) { return tc.b, tc.found, tc.err }}
			g := &Gate{Tokens: r.tokens, Bindings: sb, Leases: r.st}
			c := call(lease, tok, ep)
			c.Session, c.Reconcile = "sess-2", true
			err := g.Check(context.Background(), c)
			switch {
			case tc.err != nil:
				if !errors.Is(err, boom) {
					t.Fatalf("err = %v", err)
				}
			case tc.reason == "":
				if err != nil {
					t.Fatalf("err = %v", err)
				}
			default:
				wantDenial(t, err, tc.reason)
			}
		})
	}
}

func TestReleaseClearsThePendingCancelAndBoundsTheFinishedMemory(t *testing.T) {
	d := &Driver{queues: map[string][]protocol.Lease{}, finished: map[string]bool{}, pending: map[string]map[string]string{"n-1": {"l1": "why", "l2": "why"}}}
	d.release(&route{node: "n-1", leaseID: "l1", done: make(chan struct{})})
	if _, ok := d.pending["n-1"]["l1"]; ok || len(d.pending["n-1"]) != 1 {
		t.Fatalf("pending %v", d.pending["n-1"])
	}
	for i := 0; i < maxFinished-1; i++ { // with l1 the memory now holds exactly maxFinished leases
		id := "f" + strconv.Itoa(i)
		d.order = append(d.order, id)
		d.finished[id] = true
	}
	if len(d.order) != maxFinished {
		t.Fatalf("setup: %d", len(d.order))
	}
	d.release(&route{node: "n-1", leaseID: "l3", done: make(chan struct{})}) // at the bound the oldest makes room
	if len(d.order) != maxFinished || d.finished["l1"] || !d.finished["l3"] || d.order[0] != "f0" || d.order[maxFinished-1] != "l3" {
		t.Fatalf("order %d, l1 %v, l3 %v", len(d.order), d.finished["l1"], d.finished["l3"])
	}
}

func TestRetryCancelsKeepsWhatCouldNotBeSent(t *testing.T) {
	d := &Driver{pending: map[string]map[string]string{"n-1": {"ok": "w", "bad": "w"}}}
	d.Cancel = func(node, id, why string) error {
		if id == "bad" {
			return errors.New("no control stream")
		}
		return nil
	}
	d.retryCancels("n-1")
	if len(d.pending["n-1"]) != 1 || d.pending["n-1"]["bad"] != "w" {
		t.Fatalf("pending %v", d.pending["n-1"])
	}
}

func TestForwardAcceptsOnlyAHigherLineSeq(t *testing.T) {
	d := &Driver{}
	rt := &route{}
	for _, c := range []struct {
		seq  uint64
		want bool
	}{{5, true}, {5, false}, {4, false}, {6, true}} {
		if got := d.forward(rt, c.seq); got != c.want {
			t.Fatalf("forward(%d) = %v, want %v", c.seq, got, c.want)
		}
	}
}

func brokenLog(t *testing.T) *LogWriter {
	t.Helper()
	lw, err := OpenLog(LogFile(t.TempDir(), "l"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lw.Append(logOf(1, "x")); err != nil {
		t.Fatal(err)
	}
	lw.mu.Lock()
	_ = lw.f.Close()
	lw.mu.Unlock()
	return lw
}

func TestSyncLogsReportsAFailureAndOnlyAFailure(t *testing.T) {
	good, err := OpenLog(LogFile(t.TempDir(), "g"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = good.Close() })
	_, _ = good.Append(logOf(1, "x"))
	d := &Driver{routes: map[string]*route{"g": {lw: good}}}
	if err = d.syncLogs(); err != nil {
		t.Fatal(err)
	}
	d.routes["b"] = &route{lw: brokenLog(t)}
	_, _ = good.Append(logOf(2, "y"))
	if err = d.syncLogs(); err == nil {
		t.Fatal("a failing log must fail the sync")
	}
}

type spendFails struct{ Bindings }

func (spendFails) TokenSpend(context.Context, string) error { return errors.New("db locked") }

func TestResultLogsEachFailedSpendAttemptAndFailsTheFrame(t *testing.T) {
	for _, failing := range []bool{false, true} {
		r := newRig(t)
		lease, att, ep := r.seedLease("1", "n-1")
		var b Bindings = r.st
		if failing {
			b = spendFails{r.st}
		}
		logged := 0
		d := &Driver{Gate: &Gate{Tokens: r.tokens, Bindings: b, Leases: r.st}, Minter: r.tokens, Leases: r.st, Poll: 10 * time.Millisecond, Logf: func(string, ...any) { logged++ }}
		done := make(chan error, 1)
		go func() {
			done <- d.Run(context.Background(), nodeCap("n-1"), protoLease(lease, att, ep), &runSink{eng: r.eng})
		}()
		eventually(t, 3*time.Second, "queued", func() bool { l, _ := d.Next(context.Background(), "n-1"); return l != nil })
		res := protocol.Result{LeaseID: lease, Epoch: ep, Evidence: []byte(`{}`), Signature: protocol.Signature{KeyID: "k", Value: "v"}}
		err := d.Sink().Result(context.Background(), res)
		if failing {
			if err == nil {
				t.Fatal("a spend that never succeeds must fail the Result frame")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
		}
		if want := map[bool]int{false: 0, true: spendAttempts}[failing]; logged != want {
			t.Fatalf("failing=%v: %d log lines, want %d", failing, logged, want)
		}
	}
}

func TestSetLiveSameGenerationWakesNobody(t *testing.T) {
	c := bareCarrier(t)
	c.setLive(2)
	c.mu.Lock()
	ch := c.up
	c.mu.Unlock()
	c.setLive(2)
	c.setLive(1)
	select {
	case <-ch:
		t.Fatal("setLive with a generation that is not newer woke the waiters")
	default:
	}
}

func TestSweepTokensKeepsATokenUntilTheGraceIsOver(t *testing.T) {
	c := bareCarrier(t)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	add := func() {
		c.tokens["l1"], c.epochs["l1"], c.tokenAt["l1"] = "t", 5, at
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	add()
	c.sweepTokens(beat(), at.Add(tokenGrace-time.Nanosecond))
	if _, ok := c.tokens["l1"]; !ok {
		t.Fatal("swept inside the grace")
	}
	c.sweepTokens(beat(), at.Add(tokenGrace))
	if _, ok := c.tokens["l1"]; ok || len(c.epochs) != 0 || len(c.tokenAt) != 0 {
		t.Fatal("not swept once the grace is over")
	}
}

// --- upload.go -------------------------------------------------------------------------------------------------

func conflict(last string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(agentapi.HeaderLastSeq, last)
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":"E653","reason":"resume_seq"}`)
	}
}

func uploadCarrier(t *testing.T, o Options) *Carrier {
	t.Helper()
	c := NewCarrier(CarrierConfig{Client: o})
	t.Cleanup(func() { _ = c.Close() })
	c.mu.Lock()
	c.sess, c.nextT = "sess-1", 9
	c.mu.Unlock()
	return c
}

func TestOpenAdoptsTheCoordinatorsPositionTwiceAtMost(t *testing.T) {
	var n atomic.Int32
	o := rawOptions(t, &scriptedServer{}, func(w http.ResponseWriter, r *http.Request) { n.Add(1); conflict("3")(w, r) })
	c := uploadCarrier(t, o)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := c.open(ctx, 1, nil)
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusConflict || n.Load() != 3 {
		t.Fatalf("err %v after %d requests, want the 409 after 3", err, n.Load())
	}
	if c.confirmedSeq() != 3 {
		t.Fatalf("confirmed %d", c.confirmedSeq())
	}
}

func TestOpenRefusesAPositionItCannotAdopt(t *testing.T) {
	for name, last := range map[string]string{"not a number": "abc", "beyond what was sent": "99", "behind what was confirmed": "1"} {
		t.Run(name, func(t *testing.T) {
			var n atomic.Int32
			o := rawOptions(t, &scriptedServer{}, func(w http.ResponseWriter, r *http.Request) { n.Add(1); conflict(last)(w, r) })
			c := uploadCarrier(t, o)
			c.mu.Lock()
			c.confirmed = 2
			c.mu.Unlock()
			if _, _, err := c.open(context.Background(), 1, nil); !errors.Is(err, errResync) || n.Load() != 1 {
				t.Fatalf("err %v after %d requests", err, n.Load())
			}
		})
	}
}

func TestOpenRetriesOnceThePositionIsAdopted(t *testing.T) {
	var n atomic.Int32
	o := rawOptions(t, &scriptedServer{}, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			conflict("4")(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	c := uploadCarrier(t, o)
	resp, pw, err := c.open(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	pw.Close()
	if c.confirmedSeq() != 4 || n.Load() != 2 {
		t.Fatalf("confirmed %d after %d requests", c.confirmedSeq(), n.Load())
	}
}

func ackFrames(t *testing.T, seqs ...uint64) []byte {
	var b []byte
	for i, s := range seqs {
		b = append(b, frameBytes(t, "ack", uint64(i+1), protocol.Ack{AckSeq: s})...)
	}
	return b
}

func TestReadStreamDeliversEveryAnswerAndEndsWithTheStreamEnd(t *testing.T) {
	c := bareCarrier(t)
	body := append(ackFrames(t, 0, 0), frameBytes(t, "ack", 3, protocol.Ack{AckSeq: 0, LeaseID: "l1"})...)
	err := c.readStream(context.Background(), bytes.NewReader(body))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want the end of the stream", err)
	}
	if len(c.inbox) != 2 { // the third ack names a lease: it trims, it is not a heartbeat for the session
		t.Fatalf("%d messages delivered, want 2", len(c.inbox))
	}
	if m := <-c.inbox; m.Type != "heartbeat" || m.Ack == nil {
		t.Fatalf("message %+v", m)
	}
}

func TestReadStreamStopsWhenTheSessionTakesNothing(t *testing.T) {
	c := bareCarrier(t)
	for len(c.inbox) < cap(c.inbox) {
		c.inbox <- protocol.Message{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.readStream(ctx, bytes.NewReader(ackFrames(t, 0))); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamOnceEndsWithTheStreamEndOrTheCoordinatorsReject(t *testing.T) {
	var sendReject atomic.Bool
	srv := &scriptedServer{}
	o := rawOptions(t, srv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if sendReject.Load() {
			_, _ = w.Write(frameBytes(t, "reject", 1, protocol.Reject{Code: "E653", Reason: "lease_not_held"}))
		}
	})
	c := uploadCarrier(t, o)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.streamOnce(ctx, 0, nil); !errors.Is(err, errStreamEnded) {
		t.Fatalf("a clean end = %v", err)
	}
	sendReject.Store(true)
	var pe *protocol.Error
	if err := c.streamOnce(ctx, 0, nil); !errors.As(err, &pe) || pe.Code != "E653" {
		t.Fatalf("a reject = %v", err)
	}
}

func TestStreamOnceRestartsForANewTokenAndStillReportsAReject(t *testing.T) {
	for _, reject := range []bool{false, true} {
		srv := &scriptedServer{}
		o := rawOptions(t, srv, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			_, _ = io.Copy(io.Discard, r.Body) // until the agent closes its side
			if reject {
				_, _ = w.Write(frameBytes(t, "reject", 1, protocol.Reject{Code: "E653", Reason: "lease_not_held"}))
			}
		})
		c := uploadCarrier(t, o)
		done := make(chan error, 1)
		go func() { done <- c.streamOnce(context.Background(), 0, nil) }()
		eventually(t, 10*time.Second, "the stream to open", c.isStreaming)
		c.mu.Lock()
		c.gen++
		c.mu.Unlock()
		end := time.After(15 * time.Second)
		for ended := false; !ended; { // the idle carrier loop may take a wake-up too: ring until the uploader looks
			c.signal(c.wake)
			select {
			case err := <-done:
				ended = true
				var pe *protocol.Error
				if reject && (!errors.As(err, &pe) || pe.Code != "E653") || !reject && err != nil {
					t.Fatalf("reject=%v: err = %v", reject, err)
				}
			case <-time.After(20 * time.Millisecond):
			case <-end:
				t.Fatal("streamOnce did not end after the token set changed")
			}
		}
	}
}

type lockedBytes struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBytes) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBytes) lines() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Count(l.b.Bytes(), []byte("\n"))
}

func TestWriteFramesNumbersEachFrameAndRemembersTheHighestWritten(t *testing.T) {
	c := bareCarrier(t)
	for i := uint64(1); i <= 2; i++ {
		if err := c.Send(logFrame("l1", i)); err != nil {
			t.Fatal(err)
		}
	}
	w := &lockedBytes{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.writeFrames(ctx, w, 0) }()
	eventually(t, 5*time.Second, "two frames on the wire", func() bool { return w.lines() == 2 })
	c.mu.Lock()
	written := c.written
	c.mu.Unlock()
	if written != 2 {
		t.Fatalf("written = %d, want 2", written)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

type hookBindings struct {
	Bindings
	onBind func()
}

func (h *hookBindings) TokenBind(ctx context.Context, b store.TokenBinding) (store.TokenBinding, bool, error) {
	got, created, err := h.Bindings.TokenBind(ctx, b)
	if h.onBind != nil {
		h.onBind()
	}
	return got, created, err
}

func TestGateDoesNotCacheABindingThatOverlappedARebind(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	hb := &hookBindings{Bindings: r.st}
	g := &Gate{Tokens: r.tokens, Bindings: hb, Leases: r.st}
	hb.onBind = func() { g.forgetOtherSessions("jti-1", "sess-2") } // a reconnect moved the token while this call was binding
	if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	n := len(g.known)
	g.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d bindings cached; the session that overlapped the rebind must prove itself again", n)
	}
	hb.onBind = nil
	if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	n = len(g.known)
	g.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d bindings cached after an undisturbed check", n)
	}
}

func TestArtifactAndCacheRefsPutTheLogOnStableStorageFirst(t *testing.T) {
	u := newUnit(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u.run(ctx, "n-1", "l1")
	u.waitQueued(t, "n-1")
	s := u.d.Sink()
	dirty := func() bool {
		u.d.mu.Lock()
		lw := u.d.routes["l1"].lw
		u.d.mu.Unlock()
		lw.mu.Lock()
		defer lw.mu.Unlock()
		return lw.dirty
	}
	for i, ref := range []func() error{
		func() error {
			return s.ArtifactRef(ctx, protocol.ArtifactRef{LeaseID: "l1", Path: "a", SHA256: "x", Size: 1})
		},
		func() error { return s.CacheRef(ctx, protocol.CacheRef{LeaseID: "l1", Spec: "s", Key: "k"}) },
	} {
		if err := s.Log(ctx, protocol.Log{LeaseID: "l1", LineSeq: uint64(i + 1), Stream: "stdout", Text: "x"}); err != nil {
			t.Fatal(err)
		}
		if !dirty() {
			t.Fatal("a log line is synced on its own; it must wait for an acknowledgement")
		}
		if err := ref(); err != nil {
			t.Fatal(err)
		}
		if dirty() {
			t.Fatalf("reference %d was forwarded with unsynced log lines before it", i)
		}
	}
}

func TestBlobSetExistsOnlyWhileTheRouteIsRegistered(t *testing.T) {
	d := &Driver{}
	rt := &route{node: "n-1", leaseID: "l1", done: make(chan struct{})}
	if err := d.enqueue(rt, protoLease("l1", "a1", 5)); err != nil {
		t.Fatal(err)
	}
	if d.blobSetFor(rt) == nil || d.blobSetFor(rt) != rt.blobs {
		t.Fatal("no blob set for a live route")
	}
	d.release(rt)
	if d.blobSetFor(rt) != nil {
		t.Fatal("a blob set for a released route would open a writer nobody discards")
	}
}

func TestAClientWithoutAPinnedRootTrustsNobody(t *testing.T) {
	cl := newClient(Options{BaseURL: "https://127.0.0.1:1/"})
	t.Cleanup(cl.close)
	tr, ok := cl.hc.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil || !tr.TLSClientConfig.RootCAs.Equal(x509.NewCertPool()) || tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("transport %+v", tr)
	}
}

func TestReleaseRemovesOnlyItsOwnLeaseFromTheQueue(t *testing.T) {
	d := &Driver{}
	r1 := &route{node: "n-1", leaseID: "l1", done: make(chan struct{})}
	r2 := &route{node: "n-1", leaseID: "l2", done: make(chan struct{})}
	for _, r := range []*route{r1, r2} {
		if err := d.enqueue(r, protoLease(r.leaseID, "a-"+r.leaseID, 5)); err != nil {
			t.Fatal(err)
		}
	}
	d.release(r1)
	l, _ := d.Next(context.Background(), "n-1")
	if l == nil || l.LeaseID != "l2" {
		t.Fatalf("next lease %+v, want l2 (l1 was released before delivery)", l)
	}
	if again, _ := d.Next(context.Background(), "n-1"); again != nil {
		t.Fatalf("a released lease was delivered: %+v", again)
	}
}
