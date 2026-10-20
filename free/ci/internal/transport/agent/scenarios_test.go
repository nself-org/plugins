package agent

// Purpose: the acceptance scenarios of the pull carrier (P7-NODE-08), written once and run both as plain tests
//          and as subtests of the integration test TestAgentPull.
// Inputs:  the e2e rig (real listener, store, CA, gate, driver) and a Carrier as the agent.
// Constraints: bounded waits; counts are exact (no duplicate or missing line, one result).

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// session drives one lease through a carrier the way the agent session would.
type leaseRun struct {
	t     *testing.T
	e     *e2e
	c     *Carrier
	l     protocol.Lease
	done  <-chan error
	seq   uint64
	token string
}

// startLease seeds an attempt, starts Driver.Run for it and waits for the agent to receive the lease.
func (e *e2e) startLease(t *testing.T, c *Carrier, node string, n string) *leaseRun {
	t.Helper()
	leaseID, att, ep := e.seedLease(n, node)
	done := e.runLease(context.Background(), node, leaseID, att, ep)
	l := *recvType(t, c, "lease", 5*time.Second).Body.(*protocol.Lease)
	return &leaseRun{t: t, e: e, c: c, l: l, done: done, token: l.Token}
}

func (r *leaseRun) log(text string) {
	r.t.Helper()
	r.seq++
	if err := r.c.Send(logMsg(r.l, r.seq, text)); err != nil {
		r.t.Fatal(err)
	}
}

func (r *leaseRun) beat() {
	r.t.Helper()
	if err := r.c.Send(beatMsg(r.l)); err != nil {
		r.t.Fatal(err)
	}
}

func (r *leaseRun) finish() {
	r.t.Helper()
	if err := r.c.Send(resultMsg(r.l)); err != nil {
		r.t.Fatal(err)
	}
	select {
	case err := <-r.done:
		if err != nil {
			r.t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		r.t.Fatal("Run did not return after the result")
	}
}

// wantLines asserts the run saw exactly lines "line-1" .. "line-n", each once, in order.
func wantLines(t *testing.T, s *runSink, n int) {
	t.Helper()
	eventually(t, 10*time.Second, fmt.Sprintf("%d log lines", n), func() bool { return len(s.lines()) >= n })
	time.Sleep(100 * time.Millisecond) // let a duplicate show up
	got := s.lines()
	if len(got) != n {
		t.Fatalf("%d lines reached the run, want %d: %v", len(got), n, got)
	}
	for i, g := range got {
		if g != fmt.Sprintf("line-%d", i+1) {
			t.Fatalf("line %d is %q: %v", i+1, g, got)
		}
	}
}

// scenarioPartition: a partition while the job logs and finishes; everything arrives once after healing.
func scenarioPartition(t *testing.T, partitionFor time.Duration) {
	e := newE2E(t, "")
	e.addNode("n-1")
	px := newNetProxy(t, e.ln.Addr().String())
	e.base = "https://" + px.addr()
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	run := e.startLease(t, c, "n-1", "1")
	for i := 1; i <= 5; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	wantLines(t, e.sink, 5)

	px.partition()
	for i := 6; i <= 15; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	run.beat()
	time.Sleep(partitionFor)
	px.heal()
	for i := 16; i <= 20; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	wantLines(t, e.sink, 20)
	run.finish()
	if n := e.sink.nResults(); n != 1 {
		t.Fatalf("%d results reached the run, want 1", n)
	}
}

// scenarioRestart: the coordinator is killed mid-stream and comes back with no memory of the session; the agent
// says hello again, resends what was not acknowledged, and no line is duplicated or lost.
func scenarioRestart(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	addr := e.ln.Addr().String()
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	run := e.startLease(t, c, "n-1", "1")
	for i := 1; i <= 8; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	cl, err := e.tokens.Verify(run.token)
	if err != nil {
		t.Fatal(err)
	}
	before, found, _ := e.st.TokenCheck(context.Background(), cl.JTI)
	if !found {
		t.Fatal("the token was never bound")
	}
	for i := 9; i <= 12; i++ { // sent after the last ack: the agent still holds them
		run.log(fmt.Sprintf("line-%d", i))
	}
	e.kill()
	for i := 13; i <= 16; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	e.start(addr) // a new listener: every session of the old one is gone
	wantLines(t, e.sink, 16)
	after, _, _ := e.st.TokenCheck(context.Background(), cl.JTI)
	if after.Session == before.Session || after.Serial != before.Serial {
		t.Fatalf("the token must move to the new session of the same certificate: before %+v after %+v", before, after)
	}
	run.finish()
	if n := e.sink.nResults(); n != 1 {
		t.Fatalf("%d results reached the run, want 1", n)
	}
	b := readLog(t, LogFile(e.drv.LogDir, run.l.LeaseID))
	if strings.Count(b, "\n") != 16 {
		t.Fatalf("the durable log has %d lines, want 16:\n%s", strings.Count(b, "\n"), b)
	}
}

// scenarioWireRefusals: the same token is accepted for every call of its session; a forged, missing, other
// lease's or other node's token is refused with E661 and the agent is told to stop that lease.
func scenarioWireRefusals(t *testing.T) {
	e := newE2E(t, "")
	c := e.connect("n-1")
	run := e.startLease(t, c, "n-1", "1")
	run.log("line-1")
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	wantLines(t, e.sink, 1)
	// a second lease of the same node: its token must not work for the first lease
	run2 := e.startLease(t, c, "n-1", "2")
	if err := e.drv.Gate.Spend(context.Background(), mustJTI(t, e, run2.token)); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"", "forged.token.value", run2.token} {
		run.c.cfgSetToken(run.l.LeaseID, tok)
		run.log("must-not-arrive")
		cn := recvType(t, c, "cancel", 5*time.Second)
		reason := cn.Body.(*protocol.Cancel).Reason
		if !strings.HasPrefix(reason, "E661 ") || strings.Contains(reason, tok) && tok != "" {
			t.Fatalf("token %q: cancel reason %q", tok, reason)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if got := e.sink.lines(); len(got) != 1 {
		t.Fatalf("refused lines reached the run: %v", got)
	}
}

func mustJTI(t *testing.T, e *e2e, token string) string {
	t.Helper()
	cl, err := e.tokens.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	return cl.JTI
}

// cfgSetToken replaces the token the carrier presents for a lease (test seam for forged tokens).
func (c *Carrier) cfgSetToken(leaseID, tok string) {
	c.mu.Lock()
	if tok == "" {
		delete(c.tokens, leaseID)
	} else {
		c.tokens[leaseID] = tok
	}
	c.gen++
	want := c.gen
	c.mu.Unlock()
	c.signal(c.wake)
	for i := 0; i < 500; i++ {
		c.mu.Lock()
		ok := c.liveGen >= want
		c.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// partitioned starts the rig with the agent behind a proxy and one running lease.
func partitioned(t *testing.T) (*e2e, *netProxy, *Carrier, *leaseRun) {
	e := newE2E(t, "")
	e.addNode("n-1")
	px := newNetProxy(t, e.ln.Addr().String())
	e.base = "https://" + px.addr()
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	return e, px, c, nil
}

// scenarioCancelDuringPartition: a cancel issued while the agent is disconnected reaches it as soon as it is
// back, while the run still waits (control stream) and after the run gave up (the lease is no longer valid, so
// the first heartbeat is answered with a stale-epoch cancel).
func scenarioCancelDuringPartition(t *testing.T, runGivesUp bool) {
	e, px, c, _ := partitioned(t)
	leaseID, att, ep := e.seedLease("1", "n-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := e.runLease(ctx, "n-1", leaseID, att, ep)
	l := *recvType(t, c, "lease", 5*time.Second).Body.(*protocol.Lease)
	run := &leaseRun{t: t, e: e, c: c, l: l, done: done, token: l.Token}
	run.log("line-1")
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	px.partition()
	time.Sleep(100 * time.Millisecond)
	cancel() // the run is cancelled while the agent cannot be told
	if runGivesUp {
		select {
		case err := <-done:
			if ReasonOf(err) != ReasonCancelLate {
				t.Fatalf("Run = %v, want the cancel timeout", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run never gave up")
		}
		// what the dispatcher does when the carrier ends without a result
		if err := e.st.Transition(context.Background(), att, "running", "cancelled", store.TransitionDetail{Epoch: attemptEpoch(t, e, att)}); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	px.heal()
	if runGivesUp {
		run.beat() // the reconnecting agent reports the lease it kept
	}
	cn := recvType(t, c, "cancel", 5*time.Second)
	if got := cn.Body.(*protocol.Cancel); got.LeaseID != leaseID {
		t.Fatalf("cancel for %q, want %q", got.LeaseID, leaseID)
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("the cancel took %v after the agent was back", took)
	}
	if !runGivesUp {
		run.finish() // the killed job reports; Run returns
	}
}

func attemptEpoch(t *testing.T, e *e2e, attemptID string) int64 {
	t.Helper()
	_, ep, err := e.st.AttemptStateEpoch(context.Background(), attemptID)
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

// scenarioRevokeWhilePolling: a revoke ends the agent's live requests within a second and the agent cannot
// connect again.
func scenarioRevokeWhilePolling(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	cert := e.nodeCert("n-1")
	var mu sync.Mutex
	var notes []string
	c := e.carrier(cert, func(cfg *CarrierConfig) {
		cfg.Logf = func(f string, a ...any) { mu.Lock(); notes = append(notes, fmt.Sprintf(f, a...)); mu.Unlock() }
	})
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	time.Sleep(200 * time.Millisecond) // the long-poll and the control stream are open
	serial := hex.EncodeToString(cert.Leaf.SerialNumber.Bytes())
	started := time.Now()
	if _, err := e.st.PKIDeny(context.Background(), "test", "n-1", "revoked", serial); err != nil {
		t.Fatal(err)
	}
	e.srv.CloseNode("n-1")
	eventually(t, 3*time.Second, "the agent to notice the closed connections", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(notes) > 0
	})
	if took := time.Since(started); took > time.Second+500*time.Millisecond {
		t.Fatalf("the live requests ended after %v, want within 1 s", took)
	}
	time.Sleep(600 * time.Millisecond) // several reconnect attempts
	mu.Lock()
	defer mu.Unlock()
	for _, n := range notes {
		if strings.Contains(n, "welcome") {
			t.Fatalf("a revoked node was welcomed again: %s", n)
		}
	}
	if len(e.srv.NodesConnected()) != 0 {
		t.Fatalf("revoked node still connected: %v", e.srv.NodesConnected())
	}
}

// scenarioServerLeafKinds: the agent connects only to a coordinator-kind server leaf; a node or user leaf that
// chains to the pinned root is refused at the handshake.
func scenarioServerLeafKinds(t *testing.T) {
	r := newRig(t)
	e := &e2e{rig: r}
	node := e.nodeCertOf("n-1")
	user, err := r.ca.IssueUser(context.Background(), "alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	userPair, err := tls.X509KeyPair(append(append([]byte{}, user.CertPEM...), user.RootPEM...), user.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	coord, err := r.ca.IssueServer("coord", nil, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, leaf := range map[string]tls.Certificate{"node": node, "user": userPair, "coordinator": coord} {
		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{leaf}, MinVersion: tls.VersionTLS13})
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func() { _, _ = io.Copy(io.Discard, c); _ = c.Close() }()
			}
		}()
		cfg, _ := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), ServerName: "127.0.0.1"})
		conn, err := tls.Dial("tcp", ln.Addr().String(), cfg)
		if conn != nil {
			_ = conn.Close()
		}
		_ = ln.Close()
		if name == "coordinator" && err != nil {
			t.Fatalf("a coordinator leaf was refused: %v", err)
		}
		if name != "coordinator" && err == nil {
			t.Fatalf("the agent accepted a %s-kind server leaf that chains to the pinned root", name)
		}
	}
}

func blobFrames(l protocol.Lease, data []byte, chunk int, from int, flip bool) []protocol.Message {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ep := l.Epoch
	out := []protocol.Message{{Type: "digest", LeaseID: l.LeaseID, Epoch: &ep, Body: protocol.Digest{Stream: "artifact", SHA256: digest, Size: int64(len(data))}}}
	for off := from; off < len(data); off += chunk {
		end := min(off+chunk, len(data))
		part := append([]byte(nil), data[off:end]...)
		if flip && off == from {
			part[0] ^= 0x01
		}
		out = append(out, protocol.Message{Type: "chunk", LeaseID: l.LeaseID, Epoch: &ep, Body: protocol.Chunk{Stream: "artifact", Offset: int64(off), Data: base64.StdEncoding.EncodeToString(part), Digest: digest, Final: end == len(data)}})
	}
	return out
}

func blobData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

// scenarioBlob: a good blob is committed; one with a flipped byte is rejected by digest and fails the lease; an
// interrupted blob resumes from the offset the coordinator reports.
func scenarioBlob(t *testing.T) {
	e := newE2E(t, "")
	c := e.connect("n-1")
	run := e.startLease(t, c, "n-1", "1")
	data := blobData(10_000)
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	// interrupted: the first 4000 bytes, then the agent announces again and learns the offset
	frames := blobFrames(run.l, data, 1000, 0, false)
	for _, f := range frames[:5] { // digest + 4 chunks
		if err := c.Send(f); err != nil {
			t.Fatal(err)
		}
	}
	off := recvType(t, c, "offset", 5*time.Second).Body.(*protocol.Offset)
	if off.Offset != 0 || off.SHA256 != digest {
		t.Fatalf("first offset = %+v", off)
	}
	eventually(t, 5*time.Second, "4000 bytes held", func() bool {
		e.sink.mu.Lock()
		defer e.sink.mu.Unlock()
		b := e.sink.blobs[digest]
		return b != nil && len(b.data) == 4000
	})
	if err := c.Send(frames[0]); err != nil { // announce again
		t.Fatal(err)
	}
	off = recvType(t, c, "offset", 5*time.Second).Body.(*protocol.Offset)
	if off.Offset != 4000 {
		t.Fatalf("resume offset = %d, want 4000", off.Offset)
	}
	for _, f := range blobFrames(run.l, data, 1000, int(off.Offset), false)[1:] {
		if err := c.Send(f); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, 5*time.Second, "the blob to commit", func() bool {
		e.sink.mu.Lock()
		defer e.sink.mu.Unlock()
		return e.sink.blobs[digest].committed
	})
	if !bytes.Equal(e.sink.blobs[digest].data, data) {
		t.Fatal("the committed blob differs from what was sent")
	}

	// corrupt: one flipped byte is caught by the digest; the lease is cancelled and Run fails
	bad := blobData(5_000)
	bad[0] ^= 0xff // another blob
	badSum := sha256.Sum256(bad)
	frames = blobFrames(run.l, bad, 1000, 0, false)
	frames[2].Body = func() any {
		b := frames[2].Body.(protocol.Chunk)
		raw, _ := base64.StdEncoding.DecodeString(b.Data)
		raw[3] ^= 0x01
		b.Data = base64.StdEncoding.EncodeToString(raw)
		return b
	}()
	for _, f := range frames {
		if err := c.Send(f); err != nil {
			t.Fatal(err)
		}
	}
	cn := recvType(t, c, "cancel", 5*time.Second).Body.(*protocol.Cancel)
	if !strings.Contains(cn.Reason, ReasonBlob) {
		t.Fatalf("cancel reason %q", cn.Reason)
	}
	select {
	case err := <-run.done:
		if ReasonOf(err) != ReasonBlob {
			t.Fatalf("Run = %v, want E656 %s", err, ReasonBlob)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not fail on the corrupt blob")
	}
	e.sink.mu.Lock()
	defer e.sink.mu.Unlock()
	if b := e.sink.blobs[hex.EncodeToString(badSum[:])]; b != nil && b.committed {
		t.Fatal("a blob that failed its digest was committed")
	}
}

// scenarioCancelRace: a cancel and a result for the same attempt race; the store's compare-and-swap lets exactly
// one win. The cancel that loses gets E605 (state changed concurrently); the result that loses gets E662 (the
// lease is gone, so the agent stops and cleans up). Never two winners and never none.
func scenarioCancelRace(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	wins := map[string]int{}
	for i := 0; i < 25; i++ {
		lease, att, ep := r.seedLease(fmt.Sprint(i), "n-1")
		aep := attemptEpoch2(t, r, att)
		var resErr, canErr error
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() { defer wg.Done(); <-start; resErr = r.eng.Result(ctx, lease, int64(ep)) }()
		go func() {
			defer wg.Done()
			<-start
			canErr = r.st.Transition(ctx, att, "running", "cancelled", store.TransitionDetail{Epoch: aep})
		}()
		close(start)
		wg.Wait()
		switch {
		case resErr == nil && canErr == nil:
			t.Fatalf("round %d: both the result and the cancel won", i)
		case resErr != nil && canErr != nil:
			t.Fatalf("round %d: neither won: result %v, cancel %v", i, resErr, canErr)
		case resErr == nil:
			wins["result"]++
			wantLoser(t, canErr, "E605") // the cancel meets a finalizing attempt: state changed concurrently
		default:
			wins["cancel"]++
			wantLoser(t, resErr, "E662") // the result meets a cancelled attempt: stale lease, the agent cleans up
		}
	}
	t.Logf("wins: %v", wins)
}

func wantLoser(t *testing.T, err error, code string) {
	t.Helper()
	var se *store.Error
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("the loser got %v, want %s", err, code)
	}
}

func attemptEpoch2(t *testing.T, r *rig, att string) int64 {
	t.Helper()
	_, ep, err := r.st.AttemptStateEpoch(context.Background(), att)
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

// scenarioTokenTable runs the whole R-DS-15 table: acceptance by call type, and every refusal with its reason.
func scenarioTokenTable(t *testing.T) {
	t.Run("one token on log, heartbeat and result", TestGateAdmitsTheSameTokenOnLogHeartbeatAndResult)
	t.Run("each refusal has its own reason", TestGateRefusalTable)
	t.Run("expired", TestGateExpiredToken)
	t.Run("terminal", TestGateTerminalAttempt)
	t.Run("spent", TestGateSpentTokenIsReplayed)
	t.Run("over the wire", func(t *testing.T) { scenarioWireRefusals(t) })
}
