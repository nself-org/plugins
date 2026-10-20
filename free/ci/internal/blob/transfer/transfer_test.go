package transfer

import (
	"context"
	"errors"
	"fmt"
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
		"file cap":    {func(r *Receiver) { r.Limits.MaxFile = 4999 }, ReasonSizeCap},
		"lease bytes": {func(r *Receiver) { r.Limits.Budget = func(context.Context, string) (int64, error) { return 4999, nil } }, ReasonSizeCap},
		"stale epoch": {func(r *Receiver) {
			r.Live = func(context.Context) error { return fmt.Errorf("%w: epoch moved", ErrStale) }
		}, ReasonStaleEpoch},
		"store fault":  {func(r *Receiver) { r.Live = func(context.Context) error { return errors.New("database is locked") } }, ReasonInterrupted},
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

func TestBudgetIsAskedPerDigestAndAHeldBlobIsFree(t *testing.T) {
	st := newStore(t)
	r := newReceiver(t, st, "lease-1")
	var asked []string
	r.Limits.Budget = func(_ context.Context, sha string) (int64, error) { asked = append(asked, sha); return 100, nil }
	a, b := pattern(60), pattern(120)
	if _, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(a), Size: 60}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(a), Size: 60}); err != nil {
		t.Fatalf("a re-announce of the same digest must not be charged against itself: %v", err)
	}
	if _, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(b), Size: 120}); !IsReason(err, ReasonSizeCap) {
		t.Fatalf("120 B against 100 B left: %v", err)
	}
	if len(asked) != 3 || asked[0] != sum(a) || asked[2] != sum(b) {
		t.Fatalf("budget asked for %v", asked)
	}
	// a blob the realm holds is answered without asking the budget at all
	if err := newWire(t, newReceiver(t, st, "lease-2")).sender().Send(context.Background(), blobOf(a)); err != nil {
		t.Fatal(err)
	}
	asked = nil
	if w, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sum(a), Size: 60}); err != nil || !w.Held() || len(asked) != 0 {
		t.Fatalf("held blob: err=%v held=%v asked=%v", err, w != nil && w.Held(), asked)
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
		return fmt.Errorf("%w: moved", ErrStale)
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
	up := &fakeUp{offs: offs, answer: true, onChunk: func(c protocol.Chunk) {
		// the receiver keeps asking for an earlier offset
		offs <- protocol.Offset{Stream: c.Stream, SHA256: c.Digest, Offset: 1}
	}}
	s := &Sender{Up: up, Offsets: offs, ChunkSize: 1024}
	err := s.Send(context.Background(), blobOf(data))
	if !IsReason(err, ReasonInterrupted) {
		t.Fatalf("err = %v, want E714 interrupted", err)
	}
	if up.digests != MaxResumes+1 {
		t.Fatalf("announced %d times, want %d passes", up.digests, MaxResumes+1)
	}
}

func TestSenderResumesAfterACutSend(t *testing.T) {
	data := pattern(5 * 1024)
	offs := make(chan protocol.Offset, 64)
	cuts := 0
	var sent []int64
	up := &fakeUp{offs: offs, answer: true}
	up.onChunkErr = func(c protocol.Chunk) error {
		if c.Offset == 2*1024 && cuts == 0 {
			cuts++
			return errors.New("connection cut")
		}
		sent = append(sent, c.Offset)
		return nil
	}
	up.first = func(pass int) int64 { return map[int]int64{1: 0, 2: 2 * 1024}[pass] }
	s := &Sender{Up: up, Offsets: offs, ChunkSize: 1024}
	if err := s.Send(context.Background(), blobOf(data)); err != nil {
		t.Fatal(err)
	}
	want := []int64{0, 1024, 2048, 3072, 4096}
	if len(sent) != len(want) {
		t.Fatalf("chunks sent = %v, want %v (resume from the reported 2048, nothing resent)", sent, want)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Fatalf("chunks sent = %v, want %v", sent, want)
		}
	}
}

func TestSenderTimesOutWithoutAnOffset(t *testing.T) {
	s := &Sender{Up: &fakeUp{}, Offsets: make(chan protocol.Offset), WaitOffset: 20 * time.Millisecond}
	if err := s.Send(context.Background(), blobOf(pattern(10))); !IsReason(err, ReasonInterrupted) {
		t.Fatalf("err = %v", err)
	}
}

func TestSenderRejectsAnImpossibleOffset(t *testing.T) {
	data := pattern(100)
	offs := make(chan protocol.Offset, 1)
	s := &Sender{Up: &fakeUp{offs: offs, answer: true, first: func(int) int64 { return 101 }}, Offsets: offs}
	if err := s.Send(context.Background(), blobOf(data)); !IsReason(err, ReasonProtocol) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Send(context.Background(), Blob{Stream: StreamArtifact, SHA256: sum(nil), Size: 0, Open: func() (io.ReadCloser, error) { return nil, nil }}); !IsReason(err, ReasonProtocol) {
		t.Fatalf("zero size: %v", err)
	}
}

// fakeUp is a carrier end: with answer set, every announce is answered with first(pass) (default 0).
type fakeUp struct {
	offs       chan protocol.Offset
	answer     bool
	first      func(pass int) int64
	onChunk    func(protocol.Chunk)
	onChunkErr func(protocol.Chunk) error
	digests    int
}

func (f *fakeUp) Digest(d protocol.Digest) error {
	f.digests++
	if f.answer {
		var off int64
		if f.first != nil {
			off = f.first(f.digests)
		}
		f.offs <- protocol.Offset{Stream: d.Stream, SHA256: d.SHA256, Offset: off}
	}
	return nil
}
func (f *fakeUp) Chunk(c protocol.Chunk) error {
	if f.onChunk != nil {
		f.onChunk(c)
	}
	if f.onChunkErr != nil {
		return f.onChunkErr(c)
	}
	return nil
}
