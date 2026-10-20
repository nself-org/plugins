package remote

// Purpose: the node fetcher against a simulated coordinator: a hit costs no frame, a miss or a
// corrupt copy is fetched, a flipped byte in transit is asked for again, a silent coordinator
// degrades the lease to local copies, and nothing outside the plan is requested.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// simCoordinator plays the coordinator pusher: a request starts a transfer, the node's answer
// streams from its position, a final chunk ends it.
type simCoordinator struct {
	data []byte
	sha  string
	in   chan protocol.Message

	mu         sync.Mutex
	offsets    []protocol.Offset
	announced  bool
	finished   bool // the final chunk of the running transfer was sent
	flip       int  // transfers whose first chunk gets a flipped byte
	dropFirst  bool // the first chunk of the first transfer is lost
	dropFinal  bool // the final chunk of the first transfer is lost
	badSize    bool // the announce names the wrong size
	chunksSent int
	transfers  int
}

func newSim(data []byte) *simCoordinator {
	sum := sha256.Sum256(data)
	return &simCoordinator{data: data, sha: hex.EncodeToString(sum[:]), in: make(chan protocol.Message, 64)}
}

func (c *simCoordinator) wire() Wire { return Wire{Offset: c.onOffset, In: c.in} }

func (c *simCoordinator) onOffset(o protocol.Offset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offsets = append(c.offsets, o)
	size := int64(len(c.data))
	switch {
	case o.Offset == size: // the node's confirmation
		c.announced, c.finished = false, false
	case !c.announced || c.finished: // a request, or the node's ask for the blob again after its re-hash failed
		c.announced, c.finished = true, false
		c.transfers++
		d := protocol.Digest{Stream: "cache", SHA256: c.sha, Size: size}
		if c.badSize {
			d.Size++
		}
		c.in <- protocol.Message{Type: "digest", Body: &d}
	default: // the answer to the announce, or a resync
		c.stream(o.Offset)
	}
	return nil
}

func (c *simCoordinator) stream(from int64) {
	const step = 64 << 10
	for pos := from; pos < int64(len(c.data)); pos += step {
		end := min(pos+step, int64(len(c.data)))
		chunk := append([]byte(nil), c.data[pos:end]...)
		if pos == from && c.flip > 0 {
			c.flip--
			chunk[0] ^= 0xff
		}
		c.chunksSent++
		if c.dropFirst && pos == 0 {
			c.dropFirst = false
			continue
		}
		c.finished = end == int64(len(c.data))
		if c.dropFinal && c.finished {
			c.dropFinal = false
			continue
		}
		c.in <- protocol.Message{Type: "chunk", Body: &protocol.Chunk{Stream: "cache", Offset: pos, Data: base64.StdEncoding.EncodeToString(chunk), Digest: c.sha, Final: end == int64(len(c.data))}}
	}
}

func planFor(sha string, size int64) *protocol.LeaseCache {
	return &protocol.LeaseCache{Namespace: "ns", Access: "ro", Restore: []protocol.CacheRestore{{Spec: "go", Key: sha[:8], Dir: "go", Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: sha, Size: size}}}}}
}

type fetchRig struct {
	t   *testing.T
	rt  *ReadThrough
	sim *simCoordinator
	f   *Fetcher
}

func newFetchRig(t *testing.T) *fetchRig {
	t.Helper()
	rt, err := NewReadThrough(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	sim := newSim(bytes.Repeat([]byte("synthetic cache bytes "), 20000)) // 440 KB: seven chunks
	f := NewFetcher(sim.wire(), planFor(sim.sha, int64(len(sim.data))))
	f.First, f.Idle = 200*time.Millisecond, 200*time.Millisecond
	return &fetchRig{t: t, rt: rt, sim: sim, f: f}
}

func (r *fetchRig) open() (*os.File, error) {
	return r.f.Open(context.Background(), r.rt, "ns", r.sim.sha, int64(len(r.sim.data)))
}

func (r *fetchRig) mustRead(f *os.File) {
	r.t.Helper()
	got, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil || !bytes.Equal(got, r.sim.data) {
		r.t.Fatalf("copy differs from the coordinator blob (err %v)", err)
	}
}

func (r *fetchRig) copies() (n int) {
	_ = filepath.WalkDir(r.rt.dir, func(_ string, d os.DirEntry, _ error) error {
		if !d.IsDir() && len(d.Name()) == 64 {
			n++
		}
		return nil
	})
	return n
}

func TestFetchPublishesAVerifiedCopyAndConfirms(t *testing.T) {
	r := newFetchRig(t)
	f, err := r.open()
	if err != nil {
		t.Fatal(err)
	}
	r.mustRead(f)
	last := r.sim.offsets[len(r.sim.offsets)-1]
	if last.Offset != int64(len(r.sim.data)) || last.SHA256 != r.sim.sha {
		t.Fatalf("no confirmation: %+v", r.sim.offsets)
	}
	if r.copies() != 1 {
		t.Fatalf("copies on disk: %d", r.copies())
	}
}

func TestFetchSecondUseIsLocalAndSendsNoFrame(t *testing.T) {
	r := newFetchRig(t)
	f, err := r.open()
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	sent, chunks := len(r.sim.offsets), r.sim.chunksSent
	f, err = r.open()
	if err != nil {
		t.Fatal(err)
	}
	r.mustRead(f)
	if len(r.sim.offsets) != sent || r.sim.chunksSent != chunks {
		t.Fatal("a valid node copy went back to the coordinator")
	}
}

func TestFetchReplacesACorruptCopy(t *testing.T) {
	r := newFetchRig(t)
	name, err := r.rt.name("ns", r.sim.sha)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), r.sim.data...)
	bad[10] ^= 1
	if err = os.WriteFile(name, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := r.open()
	if err != nil {
		t.Fatal(err)
	}
	r.mustRead(f)
	if r.sim.transfers != 1 {
		t.Fatalf("transfers %d", r.sim.transfers)
	}
}

func TestFetchAsksAgainAfterAFlippedByte(t *testing.T) {
	r := newFetchRig(t)
	r.sim.flip = 1
	f, err := r.open()
	if err != nil {
		t.Fatal(err)
	}
	r.mustRead(f)
	if r.sim.transfers != 2 {
		t.Fatalf("transfers %d", r.sim.transfers)
	}
}

func TestFetchGivesUpOnATransferThatNeverVerifies(t *testing.T) {
	r := newFetchRig(t)
	r.sim.flip = 100
	_, err := r.open()
	if !errors.Is(err, errTransit) {
		t.Fatalf("err %v", err)
	}
	if r.copies() != 0 {
		t.Fatal("an unverified blob was published")
	}
	if r.sim.transfers != maxTransit+1 {
		t.Fatalf("transfers %d", r.sim.transfers)
	}
}

func TestFetchResyncsAfterALostChunk(t *testing.T) {
	r := newFetchRig(t)
	r.sim.dropFirst = true
	f, err := r.open()
	if err != nil {
		t.Fatal(err)
	}
	r.mustRead(f)
	if r.sim.transfers > 2 {
		t.Fatalf("a lost chunk cost %d transfers", r.sim.transfers)
	}
}

func TestFetchRefusesAnAnnounceThatContradictsThePlan(t *testing.T) {
	r := newFetchRig(t)
	r.sim.badSize = true
	if _, err := r.open(); err == nil {
		t.Fatal("a wrong announced size was accepted")
	}
	if r.copies() != 0 {
		t.Fatal("published")
	}
}

func TestFetchNeverAsksForADigestOutsideThePlan(t *testing.T) {
	r := newFetchRig(t)
	other := bytes.Repeat([]byte("x"), 100)
	sum := sha256.Sum256(other)
	if _, err := r.f.Open(context.Background(), r.rt, "ns", hex.EncodeToString(sum[:]), 100); err == nil {
		t.Fatal("opened a blob outside the plan")
	}
	if len(r.sim.offsets) != 0 {
		t.Fatalf("frames sent for an unplanned digest: %+v", r.sim.offsets)
	}
}

func TestFetchIgnoresFramesOfOtherBlobsAndUnannouncedChunks(t *testing.T) {
	r := newFetchRig(t)
	stray := protocol.Message{Body: &protocol.Chunk{Stream: "cache", Offset: 0, Data: "AAAA", Digest: r.sim.sha}}
	r.sim.in <- stray // a chunk before any announce: dropped by the drain, never written
	f, err := r.open()
	if err != nil {
		t.Fatal(err)
	}
	r.mustRead(f)
}

func TestFetchSilentCoordinatorDegradesTheWholeLease(t *testing.T) {
	r := newFetchRig(t)
	silent := Wire{Offset: func(protocol.Offset) error { return nil }, In: r.sim.in}
	r.f = NewFetcher(silent, planFor(r.sim.sha, int64(len(r.sim.data))))
	r.f.First = 30 * time.Millisecond
	start := time.Now()
	if _, err := r.open(); !errors.Is(err, ErrNoRestore) {
		t.Fatalf("err %v", err)
	}
	if _, err := r.open(); !errors.Is(err, ErrNoRestore) || time.Since(start) > 2*time.Second {
		t.Fatalf("second open waited again: %v after %s", err, time.Since(start))
	}
}

func TestNilFetcherServesLocalCopiesOnly(t *testing.T) {
	r := newFetchRig(t)
	var none *Fetcher
	if _, err := none.Open(context.Background(), r.rt, "ns", r.sim.sha, int64(len(r.sim.data))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err %v", err)
	}
	if NewFetcher(Wire{}, planFor(r.sim.sha, 1)) != nil {
		t.Fatal("a fetcher without a channel")
	}
}

func TestFetchAsksAgainWhenTheFinalChunkIsLost(t *testing.T) {
	r := newFetchRig(t)
	r.sim.dropFinal = true
	r.f.Idle = 60 * time.Millisecond
	f, err := r.open()
	if err != nil {
		t.Fatalf("a lost final chunk failed the fetch: %v", err)
	}
	r.mustRead(f)
}

func TestFetchStrayFramesDoNotExtendTheWait(t *testing.T) {
	r := newFetchRig(t)
	silent := Wire{Offset: func(protocol.Offset) error { return nil }, In: r.sim.in}
	r.f = NewFetcher(silent, planFor(r.sim.sha, int64(len(r.sim.data))))
	r.f.First = 150 * time.Millisecond
	stop := make(chan struct{})
	defer close(stop)
	go func() { // frames of another digest, faster than the wait
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				select {
				case r.sim.in <- protocol.Message{Body: &protocol.Chunk{Stream: "cache", Digest: "ff", Data: "AA"}}:
				default:
				}
			}
		}
	}()
	start := time.Now()
	if _, err := r.open(); !errors.Is(err, ErrNoRestore) || time.Since(start) > 2*time.Second {
		t.Fatalf("err=%v after %s", err, time.Since(start))
	}
}

func TestFetchRepeatedAnnouncesDoNotHoldTheLeaseForever(t *testing.T) {
	r := newFetchRig(t)
	silent := Wire{Offset: func(protocol.Offset) error { return nil }, In: r.sim.in}
	r.f = NewFetcher(silent, planFor(r.sim.sha, int64(len(r.sim.data))))
	r.f.First, r.f.Idle = 120*time.Millisecond, 120*time.Millisecond
	stop := make(chan struct{})
	defer close(stop)
	go func() { // a buggy coordinator announces again and again, never sending a byte
		for {
			select {
			case <-stop:
				return
			case <-time.After(15 * time.Millisecond):
				select {
				case r.sim.in <- protocol.Message{Body: &protocol.Digest{Stream: "cache", SHA256: r.sim.sha, Size: int64(len(r.sim.data))}}:
				default:
				}
			}
		}
	}()
	start := time.Now()
	if _, err := r.open(); !errors.Is(err, ErrNoRestore) || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %s", err, time.Since(start))
	}
}
