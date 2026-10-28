package agentapi

// Purpose: tests of the one-stream-per-session rule and of the once-per-lease refusal answer (D-1378): a restarted
//          stream takes over from a slow handler of the old one instead of decoding the same frames beside it, and a
//          lease the admit hook keeps refusing is logged and cancelled once per stream, not once per frame.
// Inputs:  the rig with a sink that blocks and counts concurrent calls, and a recording admit hook.
// Outputs: pass or fail.
// Constraints: waits are bounded; the sink never needs a real job.

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// blockingSink makes the first Log call wait (for its context when honour is true, else for release) and counts how
// many Log calls run at the same moment.
type blockingSink struct {
	*recSink
	honour  bool
	release chan struct{}
	first   atomic.Bool
	entered chan struct{}
	now     atomic.Int32
	peak    atomic.Int32
}

func newBlockingSink(rs *recSink, honour bool) *blockingSink {
	return &blockingSink{recSink: rs, honour: honour, release: make(chan struct{}), entered: make(chan struct{}, 8)}
}

func (b *blockingSink) Log(ctx context.Context, l protocol.Log) error {
	n := b.now.Add(1)
	defer b.now.Add(-1)
	for {
		p := b.peak.Load()
		if n <= p || b.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if b.first.CompareAndSwap(false, true) {
		b.entered <- struct{}{}
		if b.honour {
			<-ctx.Done()
			return ctx.Err()
		}
		<-b.release
	}
	return b.recSink.Log(ctx, l)
}

func TestARestartedStreamTakesOverFromASlowHandler(t *testing.T) {
	var bs *blockingSink
	r := newRig(t, func(c *Config) { bs = newBlockingSink(c.Sink.(*recSink), true); c.Sink = bs })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	logMsg := protocol.Message{Type: "log", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Log{LeaseID: leaseID, LineSeq: 1, Stream: "stdout", Text: "hi"}}
	u1 := a.openStream(0)
	u1.send(logMsg)
	<-bs.entered // the handler of the first stream is inside the slow sink call
	u2 := a.openStream(0)
	u2.send(logMsg)
	u2.send(heartbeat())
	if m := u2.read(); m.Type != "ack" {
		t.Fatalf("second stream answered %s, want the heartbeat ack", m.Type)
	}
	if p := bs.peak.Load(); p != 1 {
		t.Fatalf("%d sink calls ran at once, want 1", p)
	}
	r.sink.mu.Lock()
	defer r.sink.mu.Unlock()
	if len(r.sink.logs) != 1 {
		t.Fatalf("sink got %d log lines, want the resent one only", len(r.sink.logs))
	}
}

func TestAStreamIsRefusedWhileTheOldHandlerWillNotReturn(t *testing.T) {
	old := streamTakeoverWait
	streamTakeoverWait = 200 * time.Millisecond
	t.Cleanup(func() { streamTakeoverWait = old })
	var bs *blockingSink
	r := newRig(t, func(c *Config) { bs = newBlockingSink(c.Sink.(*recSink), false); c.Sink = bs })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u1 := a.openStream(0)
	u1.send(protocol.Message{Type: "log", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Log{LeaseID: leaseID, LineSeq: 1, Stream: "stdout", Text: "hi"}})
	<-bs.entered
	post := func() *http.Response {
		rq := a.req(http.MethodPost, "/v1/agent/stream", io.NopCloser(&emptyReader{}), map[string]string{HeaderLastSeq: "0"})
		resp, err := a.cl.Do(rq)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := post()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "stream_busy") {
		t.Fatalf("overlapping stream = %d %s, want 409 stream_busy", resp.StatusCode, body)
	}
	close(bs.release)
	eventually(t, 5*time.Second, "the old handler accepts its frame", func() bool {
		resp := post()
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusConflict && resp.Header.Get(HeaderLastSeq) == strconv.Itoa(1)
	})
	if p := bs.peak.Load(); p != 1 {
		t.Fatalf("%d sink calls ran at once, want 1", p)
	}
}

func TestARefusedLeaseIsLoggedAndCancelledOncePerStream(t *testing.T) {
	rec := &admitRec{err: refusal{"E661 wrong_session: the token is bound to another session"}}
	var mu sync.Mutex
	var lines []string
	r := newRig(t, func(c *Config) {
		c.Admit = rec.hook
		c.Logf = func(f string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, f)
		}
	})
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=tok")
	for i := 1; i <= 3; i++ {
		u.send(protocol.Message{Type: "log", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Log{LeaseID: leaseID, LineSeq: uint64(i), Stream: "stdout", Text: "x"}})
	}
	u.send(heartbeat(protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "running"}))
	if m := u.read(); m.Type != "cancel" {
		t.Fatalf("first answer %s, want the cancel", m.Type)
	}
	if m := u.read(); m.Type != "ack" { // no second cancel between the cancel and the heartbeat ack
		t.Fatalf("second answer %s, want the heartbeat ack", m.Type)
	}
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, l := range lines {
		if strings.Contains(l, "refused") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d refusal log lines for one lease on one stream, want 1: %q", n, lines)
	}
	r.sink.mu.Lock()
	defer r.sink.mu.Unlock()
	if len(r.sink.logs) != 0 {
		t.Fatal("a refused frame reached the sink")
	}
}
