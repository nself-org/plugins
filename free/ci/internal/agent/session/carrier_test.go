package session

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// TestStreamCarrierStalledPeer proves the real-pipe case: with nobody reading
// the pipe, Send gives up after sendTimeout instead of hanging, and Close
// returns within flushGrace even though a write is still blocked in the kernel.
func TestStreamCarrierStalledPeer(t *testing.T) {
	savedSend, savedFlush := sendTimeout, flushGrace
	sendTimeout, flushGrace = 300*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { sendTimeout, flushGrace = savedSend, savedFlush })
	rd, wr, err := os.Pipe() // nothing ever reads rd
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rd.Close(); _ = wr.Close() })
	c, err := NewStreamCarrier(strings.NewReader(""), wr, protocol.CarrierPolicy{}, false)
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", 60<<10)
	start := time.Now()
	var sendErr error
	for i := uint64(1); i < 4000 && sendErr == nil; i++ { // 60 KiB frames fill the 64 KiB pipe, then the queue
		sendErr = c.Send(protocol.Message{V: 1, Type: "log", Seq: i, Epoch: ptr(uint64(1)), LeaseID: "l", Body: protocol.Log{LeaseID: "l", LineSeq: i, Stream: "stdout", Text: big}})
	}
	if sendErr == nil {
		t.Fatal("Send never reported the stalled peer")
	}
	if time.Since(start) > 20*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
	closed := make(chan struct{})
	go func() { _ = c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close hung on a blocked write")
	}
}

// TestStreamCarrierFlushesOnClose proves frames queued before Close are written.
func TestStreamCarrierFlushesOnClose(t *testing.T) {
	var out bytes.Buffer
	var mu syncBuffer
	mu.w = &out
	c, err := NewStreamCarrier(strings.NewReader(""), &mu, protocol.CarrierPolicy{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 5; i++ {
		if err := c.Send(protocol.Message{V: 1, Type: "ack", Seq: i, Body: protocol.Ack{AckSeq: i}}); err != nil {
			t.Fatal(err)
		}
	}
	_ = c.Close()
	rd, err := protocol.SkipPreamble(bytes.NewReader(mu.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	dec := protocol.NewDecoder(rd, protocol.CarrierPolicy{})
	for i := 1; i <= 5; i++ {
		if m, err := dec.Decode(); err != nil || m.Seq != uint64(i) {
			t.Fatalf("frame %d: %v %v", i, m, err)
		}
	}
	if _, err := dec.Decode(); err != io.EOF {
		t.Fatalf("extra data: %v", err)
	}
}

// TestResumedStreamCarrierContinuesSequences proves a resumed carrier sends the
// next sequence number, not 1.
func TestResumedStreamCarrierContinuesSequences(t *testing.T) {
	var sb syncBuffer
	sb.w = &bytes.Buffer{}
	c, err := NewResumedStreamCarrier(strings.NewReader(""), &sb, protocol.CarrierPolicy{}, 41, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(protocol.Message{V: 1, Type: "ack", Seq: 42, Body: protocol.Ack{AckSeq: 7}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(protocol.Message{V: 1, Type: "ack", Seq: 1, Body: protocol.Ack{AckSeq: 7}}); err == nil {
		t.Fatal("accepted a restarted sequence")
	}
	_ = c.Close()
}

// syncBuffer is a goroutine-safe writer for the carrier's writer goroutine.
type syncBuffer struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.w.Write(p)
}
func (b *syncBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.w.Bytes()...)
}
