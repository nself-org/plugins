package agent

// Purpose: unit tests of the carrier's transport-session state without a network: renumbering after a new hello,
//          the reconciling heartbeat, token bookkeeping and sweep, the resume position and the frame buffer.
// Inputs:  a carrier that never connects (no hello is sent). Constraints: no sleeping except bounded polls.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// bareCarrier is a carrier that never connects: it waits for a hello nobody sends.
func bareCarrier(t *testing.T) *Carrier {
	t.Helper()
	c := NewCarrier(CarrierConfig{Client: Options{BaseURL: "https://127.0.0.1:1"}})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func logFrame(lease string, seq uint64) protocol.Message {
	ep := uint64(5)
	return protocol.Message{Type: "log", LeaseID: lease, Epoch: &ep, Body: protocol.Log{LeaseID: lease, LineSeq: seq, Stream: "stdout", Text: "x"}}
}

func beat(leases ...protocol.LeaseState) protocol.Message {
	if leases == nil {
		leases = []protocol.LeaseState{}
	}
	return protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: leases}}
}

func queued(t *testing.T, c *Carrier) []*outFrame {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*outFrame(nil), c.out...)
}

func TestResetTransportRenumbersDropsStaleBeatsAndAcksAndLeadsWithTheReconcileBeat(t *testing.T) {
	c := bareCarrier(t)
	for _, m := range []protocol.Message{logFrame("l1", 1), beat(protocol.LeaseState{LeaseID: "l1", Epoch: 5, State: "running"}), {Type: "ack", Body: protocol.Ack{AckSeq: 3}}, logFrame("l1", 2)} {
		if err := c.Send(m); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	for i, f := range c.out {
		f.tseq = uint64(i + 1)
	}
	c.nextT, c.confirmed, c.written = 4, 2, 4
	c.tokens["l1"], c.epochs["l1"] = "tok", 5
	c.resetTransport()
	c.mu.Unlock()
	fs := queued(t, c)
	var types []string
	for _, f := range fs {
		types = append(types, f.m.Type)
		if f.tseq != 0 {
			t.Fatalf("frame %s kept its old seq %d", f.m.Type, f.tseq)
		}
	}
	if len(fs) != 3 || types[0] != "heartbeat" || types[1] != "log" || types[2] != "log" {
		t.Fatalf("buffer after the reset = %v, want [heartbeat log log]", types)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nextT != 0 || c.confirmed != 0 || c.written != 0 {
		t.Fatalf("counters %d %d %d", c.nextT, c.confirmed, c.written)
	}
	if want := (fs[0].size + fs[1].size + fs[2].size); c.outBytes != want {
		t.Fatalf("outBytes %d, want %d", c.outBytes, want)
	}
}

func TestReconcileBeatNamesTheKeptLeasesAndTheOnesThatStartedAfterTheLastBeat(t *testing.T) {
	c := bareCarrier(t)
	c.mu.Lock()
	if c.reconcileBeat() != nil {
		t.Fatal("no leases and no beat: nothing to reconcile")
	}
	c.tokens["l-new"], c.epochs["l-new"] = "t2", 9
	b := c.reconcileBeat()
	hb := b.Body.(protocol.Heartbeat)
	if b.Type != "heartbeat" || len(hb.Leases) != 1 || hb.Leases[0] != (protocol.LeaseState{LeaseID: "l-new", Epoch: 9, State: "running"}) {
		t.Fatalf("beat from tokens only = %+v", b)
	}
	last := beat(protocol.LeaseState{LeaseID: "l-old", Epoch: 4, State: "running"}, protocol.LeaseState{LeaseID: "l-new", Epoch: 9, State: "cancelling"})
	c.lastBeat = &last
	b = c.reconcileBeat()
	hb = b.Body.(protocol.Heartbeat)
	if len(hb.Leases) != 2 || hb.Leases[1].State != "cancelling" {
		t.Fatalf("a lease the last beat names keeps its state: %+v", hb.Leases)
	}
	c.tokens["l-third"], c.epochs["l-third"] = "t3", 2
	hb = c.reconcileBeat().Body.(protocol.Heartbeat)
	if len(hb.Leases) != 3 || hb.Leases[2].LeaseID != "l-third" {
		t.Fatalf("beat = %+v", hb.Leases)
	}
	if got := len(last.Body.(protocol.Heartbeat).Leases); got != 2 {
		t.Fatalf("building the reconcile beat changed the remembered one (%d leases)", got)
	}
	empty := beat()
	c.lastBeat, c.tokens = &empty, map[string]string{}
	hb = c.reconcileBeat().Body.(protocol.Heartbeat)
	if hb.Leases == nil || len(hb.Leases) != 0 {
		t.Fatalf("an empty last beat stays an empty list, got %#v", hb.Leases)
	}
	c.mu.Unlock()
}

func TestTrimDropsConfirmedFramesAndTheTokenOfAFinishedLease(t *testing.T) {
	c := bareCarrier(t)
	for _, m := range []protocol.Message{logFrame("l1", 1), resultMsg(protoLease("l1", "a1", 5)), logFrame("l2", 1)} {
		if err := c.Send(m); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	for i, f := range c.out {
		f.tseq = uint64(i + 1)
	}
	c.tokens["l1"], c.epochs["l1"], c.tokenAt["l1"] = "t1", 5, time.Now()
	c.tokens["l2"], c.epochs["l2"], c.tokenAt["l2"] = "t2", 5, time.Now()
	c.trim(1)
	if len(c.out) != 2 || c.confirmed != 1 || c.tokens["l1"] == "" {
		t.Fatalf("after trim(1): %d frames, confirmed %d, token l1 %q (the result is not confirmed yet)", len(c.out), c.confirmed, c.tokens["l1"])
	}
	c.trim(1) // a repeat changes nothing
	c.trim(0)
	if len(c.out) != 2 {
		t.Fatal("a stale ack trimmed frames")
	}
	c.trim(2)
	if len(c.out) != 1 || c.tokens["l1"] != "" || c.epochs["l1"] != 0 || !c.tokenAt["l1"].IsZero() || c.tokens["l2"] == "" {
		t.Fatalf("after trim(2): %d frames, tokens %v epochs %v", len(c.out), c.tokens, c.epochs)
	}
	var want int
	for _, f := range c.out {
		want += f.size
	}
	if c.outBytes != want {
		t.Fatalf("outBytes %d, want %d", c.outBytes, want)
	}
	c.mu.Unlock()
}

func TestTrimNeverDropsAFrameThatWasNotNumbered(t *testing.T) {
	c := bareCarrier(t)
	_ = c.Send(logFrame("l1", 1))
	c.mu.Lock()
	c.trim(99)
	n := len(c.out)
	c.mu.Unlock()
	if n != 1 {
		t.Fatal("a frame that was never written was trimmed by an ack")
	}
}

func TestSweepTokensKeepsWhatIsNamedBusyOrYoung(t *testing.T) {
	c := bareCarrier(t)
	old := time.Now().Add(-2 * tokenGrace)
	_ = c.Send(logFrame("busy", 1))
	c.mu.Lock()
	for _, id := range []string{"named", "busy", "young", "gone"} {
		c.tokens[id], c.epochs[id] = "t", 1
		c.tokenAt[id] = old
	}
	c.tokenAt["young"] = time.Now().Add(-tokenGrace / 2)
	c.sweepTokens(beat(protocol.LeaseState{LeaseID: "named", Epoch: 1, State: "running"}), time.Now())
	got := map[string]bool{}
	for id := range c.tokens {
		got[id] = true
	}
	_, hasEpoch := c.epochs["gone"]
	_, hasAt := c.tokenAt["gone"]
	c.mu.Unlock()
	if !got["named"] || !got["busy"] || !got["young"] || got["gone"] || hasEpoch || hasAt {
		t.Fatalf("tokens after the sweep = %v (epoch kept %v, time kept %v)", got, hasEpoch, hasAt)
	}
	c.mu.Lock()
	c.sweepTokens(protocol.Message{Type: "heartbeat", Body: &protocol.Heartbeat{}}, time.Now()) // a pointer body names nothing
	c.mu.Unlock()
}

func TestAdoptTakesOnlyAPositionBetweenWhatIsConfirmedAndWhatWasSent(t *testing.T) {
	c := bareCarrier(t)
	c.mu.Lock()
	c.confirmed, c.nextT = 3, 7
	c.mu.Unlock()
	for last, want := range map[uint64]bool{2: false, 3: true, 5: true, 7: true, 8: false} {
		c.mu.Lock()
		c.confirmed = 3
		c.mu.Unlock()
		if got := c.adopt(last); got != want {
			t.Errorf("adopt(%d) = %v, want %v", last, got, want)
		}
	}
	c.mu.Lock()
	c.confirmed = 3
	c.mu.Unlock()
	if c.adopt(6); c.confirmedSeq() != 6 {
		t.Fatalf("confirmed = %d after adopting 6", c.confirmedSeq())
	}
}

func TestTokenSnapshotIsACopy(t *testing.T) {
	c := bareCarrier(t)
	c.mu.Lock()
	c.tokens["l1"] = "t1"
	c.gen = 4
	c.mu.Unlock()
	gen, toks := c.tokenSnapshot()
	toks["l1"] = "changed"
	toks["l9"] = "x"
	gen2, toks2 := c.tokenSnapshot()
	if gen != 4 || gen2 != 4 || toks2["l1"] != "t1" || len(toks2) != 1 {
		t.Fatalf("snapshot %d %v / %d %v", gen, toks, gen2, toks2)
	}
}

func TestRegisterTokenWaitsUntilTheStreamCarriesIt(t *testing.T) {
	c := bareCarrier(t)
	l := protoLease("l1", "a1", 7)
	l.Token = "tok-1"
	done := make(chan error, 1)
	go func() { done <- c.registerToken(context.Background(), l) }()
	select {
	case err := <-done:
		t.Fatalf("registerToken returned before the stream carried the token: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	gen, toks := c.tokenSnapshot()
	if gen != 1 || toks["l1"] != "tok-1" {
		t.Fatalf("snapshot %d %v", gen, toks)
	}
	c.mu.Lock()
	if c.epochs["l1"] != 7 || c.tokenAt["l1"].IsZero() {
		t.Fatalf("epoch %d tokenAt %v", c.epochs["l1"], c.tokenAt["l1"])
	}
	c.mu.Unlock()
	c.setLive(1)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registerToken did not return after the stream picked the token up")
	}
	// the same token again is not a new generation
	if err := c.registerToken(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	if gen, _ = c.tokenSnapshot(); gen != 1 {
		t.Fatalf("the same token bumped the generation to %d", gen)
	}
	// no token: nothing to carry
	l.Token = ""
	l.LeaseID = "l2"
	if err := c.registerToken(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	if _, toks = c.tokenSnapshot(); toks["l2"] != "" {
		t.Fatal("a lease without a token was registered")
	}
}

func TestRegisterTokenEndsWithTheContext(t *testing.T) {
	c := bareCarrier(t)
	ctx, cancel := context.WithCancel(context.Background())
	l := protoLease("l1", "a1", 7)
	l.Token = "tok-1"
	done := make(chan error, 1)
	go func() { done <- c.registerToken(ctx, l) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("registerToken ignored the cancelled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registerToken did not return")
	}
}

func TestSetLiveOnlyMovesForward(t *testing.T) {
	c := bareCarrier(t)
	c.setLive(3)
	c.setLive(2)
	c.mu.Lock()
	got := c.liveGen
	c.mu.Unlock()
	if got != 3 {
		t.Fatalf("liveGen = %d", got)
	}
}

func TestFlushedMeansEveryFrameWasWrittenAndNoResultWaitsForItsAck(t *testing.T) {
	c := bareCarrier(t)
	if !c.flushed() {
		t.Fatal("an empty buffer is flushed")
	}
	_ = c.Send(logFrame("l1", 1))
	if c.flushed() {
		t.Fatal("an unwritten frame is not flushed")
	}
	c.mu.Lock()
	c.out[0].tseq, c.written = 1, 1
	c.mu.Unlock()
	if !c.flushed() {
		t.Fatal("a written log frame is flushed (logs are not acknowledged one by one)")
	}
	_ = c.Send(resultMsg(protoLease("l1", "a1", 5)))
	c.mu.Lock()
	c.out[1].tseq, c.written = 2, 2
	c.mu.Unlock()
	if c.flushed() {
		t.Fatal("a result still waiting for its ack is not flushed")
	}
	c.mu.Lock()
	c.trim(2)
	c.mu.Unlock()
	if !c.flushed() {
		t.Fatal("after the ack everything is flushed")
	}
	for _, typ := range []string{"artifact-ref", "cache-ref"} {
		c.mu.Lock()
		c.out = []*outFrame{{m: protocol.Message{Type: typ}, tseq: 1}}
		c.written = 1
		c.mu.Unlock()
		if c.flushed() {
			t.Fatalf("a %s waiting for its ack is not flushed", typ)
		}
	}
}

func TestCloseDoesNotWaitWithoutAStream(t *testing.T) {
	c := bareCarrier(t)
	_ = c.Send(resultMsg(protoLease("l1", "a1", 5))) // never written: no connection
	began := time.Now()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 500*time.Millisecond {
		t.Fatalf("Close waited %v with no stream open; the fence calls Close to kill jobs", took)
	}
}

func TestSleepCtxReturnsOnTimeOrCancel(t *testing.T) {
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); err == nil {
		t.Fatal("a cancelled sleep returned nil")
	}
}
