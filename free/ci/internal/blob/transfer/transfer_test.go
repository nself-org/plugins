package transfer

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func TestSendWholeBlobPublishesVerifiedBytes(t *testing.T) {
	s := newStore(t)
	data := pattern(3*ChunkSize + 17)
	w := newWire(t, newReceiver(t, s, "lease-1"))
	if err := w.sender().Send(context.Background(), blobOf(data)); err != nil {
		t.Fatal(err)
	}
	if w.failed != nil {
		t.Fatal(w.failed)
	}
	if !held(t, s, data) {
		t.Fatal("the blob is not in the realm with its bytes")
	}
	if w.chunks != 4 {
		t.Fatalf("chunks = %d, want 4 of 512 KiB", w.chunks)
	}
}

func TestSendResumesFromTheReceiverOffset(t *testing.T) {
	s := newStore(t)
	data := pattern(2*ChunkSize + 5)
	r1 := newReceiver(t, s, "lease-1")
	wr, err := r1.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(data), Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wr.Write(data[:ChunkSize]); err != nil {
		t.Fatal(err)
	}
	_ = wr.Abort() // the sender died here
	if held(t, s, data) {
		t.Fatal("a half blob is visible")
	}
	w := newWire(t, newReceiver(t, s, "lease-1"))
	if err = w.sender().Send(context.Background(), blobOf(data)); err != nil {
		t.Fatal(err)
	}
	if w.failed != nil || !held(t, s, data) {
		t.Fatalf("failed=%v held=%v", w.failed, held(t, s, data))
	}
	if w.chunks != 2 {
		t.Fatalf("resumed send used %d chunks, want the 2 that were missing", w.chunks)
	}
}

func TestOtherLeaseNeverAppendsToThePartial(t *testing.T) {
	s := newStore(t)
	data := pattern(ChunkSize + 9)
	wr, err := newReceiver(t, s, "lease-A").Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(data), Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = wr.Write(data[:100])
	other, err := newReceiver(t, s, "lease-B").Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(data), Size: int64(len(data))})
	if err != nil || other.Offset() != 0 {
		t.Fatalf("lease B offset = %d, err %v; it must start at 0", other.Offset(), err)
	}
}

func TestFlippedByteIsNeverPublishedAndTheRetryIsClean(t *testing.T) {
	s := newStore(t)
	data := pattern(2*ChunkSize + 1)
	w := newWire(t, newReceiver(t, s, "lease-1"))
	flipped := false
	w.tamper = func(c *protocol.Chunk) {
		if !flipped && c.Offset == ChunkSize {
			flipped = true
			b := []byte(c.Data)
			if b[10] == 'A' {
				b[10] = 'B'
			} else {
				b[10] = 'A'
			}
			c.Data = string(b)
		}
	}
	if err := w.sender().Send(context.Background(), blobOf(data)); err != nil {
		t.Fatal(err)
	}
	if !IsReason(w.failed, ReasonDigest) {
		t.Fatalf("the receiver reported %v, want E714 digest_mismatch", w.failed)
	}
	if held(t, s, data) {
		t.Fatal("a corrupt blob became visible")
	}
	w2 := newWire(t, newReceiver(t, s, "lease-1"))
	if err := w2.sender().Send(context.Background(), blobOf(data)); err != nil || w2.failed != nil {
		t.Fatalf("clean retry: %v %v", err, w2.failed)
	}
	if !held(t, s, data) || w2.chunks != 3 {
		t.Fatalf("retry did not start from zero: held=%v chunks=%d", held(t, s, data), w2.chunks)
	}
}

func TestAnnounceRefusedBeforeAnyChunk(t *testing.T) {
	data := pattern(5000)
	h := transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(data), Size: int64(len(data))}
	for name, tc := range map[string]struct {
		mut    func(*Receiver)
		reason string
	}{
		"file cap":     {func(r *Receiver) { r.Limits.MaxFile = 4999 }, ReasonSizeCap},
		"lease bytes":  {func(r *Receiver) { r.Limits.Budget = func(context.Context) (int64, error) { return 4999, nil } }, ReasonSizeCap},
		"stale epoch":  {func(r *Receiver) { r.Live = func(context.Context) error { return errors.New("epoch moved") } }, ReasonStaleEpoch},
		"other stream": {func(r *Receiver) {}, ReasonProtocol},
	} {
		t.Run(name, func(t *testing.T) {
			r := newReceiver(t, newStore(t), "lease-1")
			tc.mut(r)
			hh := h
			if name == "other stream" {
				hh.Stream = "cache"
			}
			_, err := r.Open(context.Background(), hh)
			if !IsReason(err, tc.reason) {
				t.Fatalf("err = %v, want E714 %s", err, tc.reason)
			}
		})
	}
}

func TestLeaseByteBudgetCountsEveryBlob(t *testing.T) {
	r := newReceiver(t, newStore(t), "lease-1")
	r.Limits.Budget = func(context.Context) (int64, error) { return 100, nil }
	a, b := pattern(60), pattern(70)
	if _, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(a), Size: 60}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(b), Size: 70}); !IsReason(err, ReasonSizeCap) {
		t.Fatalf("second blob err = %v, want size_cap (60+70 > 100)", err)
	}
}

func TestStaleEpochMidStreamStopsTheWriteAndAuditsOncePerBlob(t *testing.T) {
	s := newStore(t)
	data := pattern(2 * ChunkSize)
	var live atomic.Bool
	live.Store(true)
	var audits atomic.Int32
	r := newReceiver(t, s, "lease-1")
	r.Live = func(context.Context) error {
		if live.Load() {
			return nil
		}
		return errors.New("moved")
	}
	r.Refused = func(context.Context, string, *Error) { audits.Add(1) }
	w := newWire(t, r)
	w.tamper = func(c *protocol.Chunk) {
		if c.Offset == ChunkSize {
			live.Store(false)
		}
	}
	_ = w.sender().Send(context.Background(), blobOf(data))
	if !IsReason(w.failed, ReasonStaleEpoch) || held(t, s, data) {
		t.Fatalf("failed=%v held=%v", w.failed, held(t, s, data))
	}
	for i := 0; i < 3; i++ {
		if _, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(data), Size: int64(len(data))}); !IsReason(err, ReasonStaleEpoch) {
			t.Fatal(err)
		}
	}
	if audits.Load() != 1 {
		t.Fatalf("audit rows = %d, want one for the blob", audits.Load())
	}
}

func TestDuplicateBlobIsAnsweredWithoutBytes(t *testing.T) {
	s := newStore(t)
	data := pattern(777)
	if err := newWire(t, newReceiver(t, s, "lease-1")).sender().Send(context.Background(), blobOf(data)); err != nil {
		t.Fatal(err)
	}
	w := newWire(t, newReceiver(t, s, "lease-2"))
	if err := w.sender().Send(context.Background(), blobOf(data)); err != nil || w.chunks != 0 {
		t.Fatalf("err=%v chunks=%d, want no chunk for a blob the realm holds", err, w.chunks)
	}
}

func TestVerifyRefusesAnotherDigestAndAbortedWriters(t *testing.T) {
	s := newStore(t)
	data := pattern(64)
	wr, err := newReceiver(t, s, "lease-1").Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(data), Size: 64})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = wr.Write(data)
	if err = wr.Verify(sum([]byte("other"))); !IsReason(err, ReasonDigest) {
		t.Fatalf("verify other = %v", err)
	}
	_ = wr.Abort()
	if err = wr.Close(); !IsReason(err, ReasonProtocol) {
		t.Fatalf("close after abort = %v", err)
	}
	if held(t, s, data) {
		t.Fatal("an aborted writer published")
	}
}

func TestSenderGivesUpAfterThreeRestarts(t *testing.T) {
	data := pattern(4 * 1024)
	offs := make(chan protocol.Offset, 64)
	up := &fakeUp{offs: offs, onChunk: func(c protocol.Chunk) {
		// the receiver keeps asking for an earlier offset
		offs <- protocol.Offset{Stream: c.Stream, SHA256: c.Digest, Offset: 1}
	}}
	up.offs <- protocol.Offset{Stream: StreamArtifact, SHA256: sum(data), Offset: 0}
	s := &Sender{Up: up, Offsets: offs, ChunkSize: 1024}
	err := s.Send(context.Background(), blobOf(data))
	if !IsReason(err, ReasonInterrupted) {
		t.Fatalf("err = %v, want E714 interrupted", err)
	}
}

func TestSenderTimesOutWithoutAnOffset(t *testing.T) {
	s := &Sender{Up: &fakeUp{offs: make(chan protocol.Offset)}, Offsets: make(chan protocol.Offset), WaitOffset: 20 * time.Millisecond}
	if err := s.Send(context.Background(), blobOf(pattern(10))); !IsReason(err, ReasonInterrupted) {
		t.Fatalf("err = %v", err)
	}
}

func TestSenderRejectsAnImpossibleOffset(t *testing.T) {
	data := pattern(100)
	offs := make(chan protocol.Offset, 1)
	offs <- protocol.Offset{Stream: StreamArtifact, SHA256: sum(data), Offset: 101}
	s := &Sender{Up: &fakeUp{offs: offs}, Offsets: offs}
	if err := s.Send(context.Background(), blobOf(data)); !IsReason(err, ReasonProtocol) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Send(context.Background(), Blob{Stream: StreamArtifact, SHA256: sum(nil), Size: 0, Open: func() (io.ReadCloser, error) { return nil, nil }}); !IsReason(err, ReasonProtocol) {
		t.Fatalf("zero size: %v", err)
	}
}

type fakeUp struct {
	offs    chan protocol.Offset
	onChunk func(protocol.Chunk)
}

func (f *fakeUp) Digest(protocol.Digest) error { return nil }
func (f *fakeUp) Chunk(c protocol.Chunk) error {
	if f.onChunk != nil {
		f.onChunk(c)
	}
	return nil
}
