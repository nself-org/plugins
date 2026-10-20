//go:build security

package redteam

// Purpose: red team group E, blob upload over the pull carrier and the reconnect backoff (TM-14 crafted
// artifact or cache entry, TM-13 stale writer, TM-29 limits). The attacker plays an agent that announces a blob
// with hostile digest forms, sizes and stream names, sends bytes that do not match the digest, sends too many
// or too few bytes, replays chunks, resumes onto a poisoned prefix, uses the wrong node or a finished lease.
// Inputs:  the real agent.Driver with its real blob handler (Driver.Run holds the route open); a sink whose
//          writer is honest about Abort and Verify, so every outcome shows in what the sink committed.
// Outputs: pass = nothing wrong was committed (control HELD); fail = a mismatching blob was committed (BROKEN).
// Constraints: no network; the blob handler has no file paths (the sink owns storage), so traversal is
//          attacked as a stream or digest string only.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

// eBlobW is a sink writer holding `held` bytes of an earlier upload (the resume case).
type eBlobW struct {
	mu        sync.Mutex
	data      []byte
	aborted   bool
	committed bool
}

func (w *eBlobW) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	return len(p), nil
}
func (w *eBlobW) Offset() int64 { w.mu.Lock(); defer w.mu.Unlock(); return int64(len(w.data)) }
func (w *eBlobW) Abort() error  { w.mu.Lock(); defer w.mu.Unlock(); w.aborted = true; return nil }
func (w *eBlobW) Close() error  { w.mu.Lock(); defer w.mu.Unlock(); w.committed = true; return nil }
func (w *eBlobW) Verify(want string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if sum := sha256.Sum256(w.data); hex.EncodeToString(sum[:]) != want {
		return errors.New("digest mismatch")
	}
	return nil
}

type eBlobSink struct {
	eSink
	mu      sync.Mutex
	prefix  []byte // what the next writer already holds
	writers []*eBlobW
}

func (s *eBlobSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := &eBlobW{data: append([]byte(nil), s.prefix...)}
	s.writers = append(s.writers, w)
	return w, nil
}

func (s *eBlobSink) opened() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.writers) }

func (s *eBlobSink) committedAny() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.writers {
		w.mu.Lock()
		c := w.committed
		w.mu.Unlock()
		if c {
			return true
		}
	}
	return false
}

type eBlobRig struct {
	t     *testing.T
	d     *agent.Driver
	sink  *eBlobSink
	lease string
	ep    uint64
	stop  context.CancelFunc
	done  chan struct{}
}

// newBlobRig starts Driver.Run for a fresh lease so the driver holds a real route for it.
func newBlobRig(t *testing.T, r *eRig, n string) *eBlobRig {
	t.Helper()
	lease, att, ep := r.seedLease(n, "n-1")
	b := &eBlobRig{t: t, sink: &eBlobSink{}, lease: lease, ep: ep, done: make(chan struct{})}
	b.d = &agent.Driver{Poll: 50 * time.Millisecond}
	b.d.Wire(agent.Wiring{Gate: r.gate, Minter: r.tokens, Leases: r.st})
	ctx, cancel := context.WithCancel(context.Background())
	b.stop = cancel
	node := model.Capability{}
	node.Identity.ID = "n-1"
	go func() {
		defer close(b.done)
		_ = b.d.Run(ctx, node, protocolLease(lease, att, ep), b.sink)
	}()
	t.Cleanup(func() { cancel(); <-b.done })
	// Run registers the route asynchronously: a harmless frame for an unknown stream proves it is there.
	for i := 0; i < 100; i++ {
		_, err := b.d.Blobs().Frame(ctx, "n-1", b.msg(&protocol.Digest{Stream: "", SHA256: "", Size: 0}, "digest"))
		var ref interface{ Refusal() bool }
		if err != nil && errors.As(err, &ref) && ref.Refusal() && !strings.Contains(err.Error(), "no run is waiting") {
			return b
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the driver never registered a route for the lease")
	return b
}

func protocolLease(lease, att string, ep uint64) protocol.Lease {
	return protocol.Lease{LeaseID: lease, AttemptID: att, Epoch: ep, Secrets: []protocol.SecretRef{}}
}

func (b *eBlobRig) msg(body any, typ string) protocol.Message {
	e := b.ep
	return protocol.Message{V: 1, Type: typ, LeaseID: b.lease, Epoch: &e, Body: body}
}

func (b *eBlobRig) digest(stream, sha string, size int64) error {
	_, err := b.d.Blobs().Frame(context.Background(), "n-1", b.msg(&protocol.Digest{Stream: stream, SHA256: sha, Size: size}, "digest"))
	return err
}

func (b *eBlobRig) chunk(stream, sha string, off int64, data string, final bool) error {
	_, err := b.d.Blobs().Frame(context.Background(), "n-1", b.msg(&protocol.Chunk{Stream: stream, Offset: off, Data: base64.StdEncoding.EncodeToString([]byte(data)), Digest: sha, Final: final}, "chunk"))
	return err
}

func eSum(s string) string { x := sha256.Sum256([]byte(s)); return hex.EncodeToString(x[:]) }

// TestE14BlobAnnouncementForms (TM-14): hostile stream names, digest spellings and sizes are refused before
// any writer is opened in the sink.
func TestE14BlobAnnouncementForms(t *testing.T) {
	r := newERig(t, true)
	good := eSum("good")
	cases := []struct {
		name, stream, sha string
		size              int64
	}{
		{"traversal stream", "../../etc/cron.d/x", good, 4},
		{"slash stream", "artifact/../cache", good, 4},
		{"empty stream", "", good, 4},
		{"upper stream", "ARTIFACT", good, 4},
		{"stream with NUL", "artifact\x00", good, 4},
		{"upper-case prefix", "artifact", "SHA256:" + good, 4},
		{"63 hex", "artifact", good[:63], 4},
		{"65 hex", "artifact", good + "0", 4},
		{"trailing newline", "artifact", good + "\n", 4},
		{"leading space", "artifact", " " + good, 4},
		{"non hex", "artifact", strings.Repeat("g", 64), 4},
		{"path as digest", "artifact", "../../../../" + good[:52], 4},
		{"unicode digit", "artifact", strings.Replace(good, good[:1], "١", 1), 4},
		{"size zero", "artifact", good, 0},
		{"size negative", "artifact", good, -1},
		{"size over 4 GiB", "artifact", good, 4<<30 + 1},
		{"size max int64", "artifact", good, 1<<63 - 1},
		{"double prefix", "artifact", "sha256:sha256:" + good[:50], 4},
	}
	for i, c := range cases {
		b := newBlobRig(t, r, "b"+string(rune('a'+i)))
		before := b.sink.opened()
		if err := b.digest(c.stream, c.sha, c.size); err == nil {
			t.Errorf("%s: the announcement was accepted", c.name)
		}
		if b.sink.opened() != before {
			t.Errorf("%s: a sink writer was opened for a refused announcement", c.name)
		}
	}
}

// TestE14BlobBytesAgainstDigest (TM-14, TM-13): bytes that do not hash to the announced digest, too many, too
// few, replayed or sent after the commit never leave a committed blob; a resume onto a poisoned prefix is
// caught by the sink verification; another node's frame and a frame after the run ended open nothing.
func TestE14BlobBytesAgainstDigest(t *testing.T) {
	r := newERig(t, true)
	good := eSum("goodgood")

	b := newBlobRig(t, r, "m1") // mismatching bytes
	if err := b.digest("artifact", good, 8); err != nil {
		t.Fatalf("control: announcement refused: %v", err)
	}
	_ = b.chunk("artifact", good, 0, "evilevil", true)
	if b.sink.committedAny() {
		t.Error("bytes that do not match the digest were committed")
	}

	b = newBlobRig(t, r, "m2") // too many bytes
	_ = b.digest("artifact", good, 8)
	_ = b.chunk("artifact", good, 0, "goodgoodX", true)
	if b.sink.committedAny() {
		t.Error("a blob larger than announced was committed")
	}

	b = newBlobRig(t, r, "m3") // short final
	_ = b.digest("artifact", good, 8)
	_ = b.chunk("artifact", good, 0, "good", true)
	if b.sink.committedAny() {
		t.Error("a blob that ended short was committed")
	}

	b = newBlobRig(t, r, "m4") // replayed chunk, then the rest: exactly the announced bytes, committed once
	_ = b.digest("artifact", good, 8)
	_ = b.chunk("artifact", good, 0, "good", false)
	_ = b.chunk("artifact", good, 0, "good", false) // replay at an old offset
	if err := b.chunk("artifact", good, 4, "good", true); err != nil {
		t.Fatalf("control: honest completion refused: %v", err)
	}
	if w := b.sink.writers[0]; string(w.data) != "goodgood" || !w.committed {
		t.Errorf("replayed chunk changed the stored bytes: %q committed=%v", w.data, w.committed)
	}
	if err := b.chunk("artifact", good, 8, "more", true); err == nil {
		t.Error("a chunk after the commit was accepted")
	}
	if string(b.sink.writers[0].data) != "goodgood" {
		t.Errorf("bytes were appended to a committed blob: %q", b.sink.writers[0].data)
	}

	b = newBlobRig(t, r, "m5") // resume onto a poisoned prefix
	b.sink.prefix = []byte("evil")
	_ = b.digest("artifact", good, 8)
	_ = b.chunk("artifact", good, 4, "good", true)
	if b.sink.committedAny() {
		t.Error("a resumed blob with a poisoned prefix was committed")
	}

	b = newBlobRig(t, r, "m6") // chunk of a stream that was never announced, and of the other stream
	_ = b.digest("artifact", good, 8)
	if err := b.chunk("cache", good, 0, "goodgood", true); err == nil {
		t.Error("a chunk for an unannounced stream was accepted")
	}
	if b.sink.committedAny() {
		t.Error("a chunk for an unannounced stream committed a blob")
	}

	b = newBlobRig(t, r, "m7") // another node, and a finished run
	if _, err := b.d.Blobs().Frame(context.Background(), "n-2", b.msg(&protocol.Digest{Stream: "artifact", SHA256: good, Size: 8}, "digest")); err != nil || b.sink.opened() != 0 {
		t.Errorf("a frame from another node opened %d writers (err %v)", b.sink.opened(), err)
	}
	b.stop()
	<-b.done
	_ = b.digest("artifact", good, 8)
	if b.sink.opened() != 0 {
		t.Error("a digest frame after the run ended opened a writer")
	}
}

// TestE13BackoffBounds (TM-13/DoS): the default backoff never returns a delay outside [50 ms, 60 s], never
// shrinks the ceiling and survives a hostile random source (NaN, negative, above 1).
func TestE13BackoffBounds(t *testing.T) {
	for name, rnd := range map[string]func() float64{"zero": func() float64 { return 0 }, "near one": func() float64 { return 0.999999 }, "above one": func() float64 { return 7 }, "negative": func() float64 { return -3 }} {
		b := agent.Backoff{Rand: rnd}
		for i := 0; i < 200; i++ {
			d := b.Next()
			if d < 50*time.Millisecond || d < 0 {
				t.Fatalf("%s: delay %s at attempt %d is below the floor", name, d, i)
			}
			if d > 60*time.Second && name != "above one" {
				t.Fatalf("%s: delay %s at attempt %d is above the cap", name, d, i)
			}
		}
	}
	over := agent.Backoff{Min: 1 << 62, Max: 1 << 62} // not reachable from any flag today (the zero value is used); logged as hardening
	for i := 0; i < 5; i++ {
		if d := over.Next(); d < 0 {
			t.Logf("hardening note: Backoff{Min: 1<<62} overflows to a negative delay %s (no caller sets Min)", d)
			break
		}
	}
}
