package remote

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// wire joins a coordinator Restore to a NodeRestorer in memory, frame for frame.
type wire struct {
	frames chan transport.BlobFrame
	offs   chan protocol.Offset
	chunks atomic.Int32
	// flip, when set, may change a chunk before the node sees it.
	flip func(*protocol.Chunk)
}

func newWire() *wire {
	return &wire{frames: make(chan transport.BlobFrame, 8), offs: make(chan protocol.Offset, 64)}
}

type wireLink struct{ w *wire }

func (l wireLink) Digest(d protocol.Digest) error {
	select {
	case l.w.frames <- transport.BlobFrame{Digest: &d}:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("wire: node not reading")
	}
}

func (l wireLink) Chunk(c protocol.Chunk) error {
	l.w.chunks.Add(1)
	if l.w.flip != nil {
		l.w.flip(&c)
	}
	select {
	case l.w.frames <- transport.BlobFrame{Chunk: &c}:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("wire: node not reading")
	}
}
func (l wireLink) Offsets() <-chan protocol.Offset { return l.w.offs }

type wireFeed struct{ w *wire }

func (f wireFeed) Frames() <-chan transport.BlobFrame { return f.w.frames }
func (f wireFeed) Offset(o protocol.Offset) error     { f.w.offs <- o; return nil }
func (f wireFeed) Close()                             {}

// coordRestoreFixture stores one blob on the coordinator and plans it as a restore.
func coordRestoreFixture(t *testing.T) (*Lease, *store.Store, []byte, protocol.CacheBlob) {
	t.Helper()
	l, st, _ := coordinatorFixture(t)
	data := bytes.Repeat([]byte("restore me "), 110000) // 1.2 MiB: three chunks
	sha, size, err := l.h.Blobs.Put(context.Background(), cacheRealm(l.b.Plan.Namespace), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := protocol.CacheBlob{Path: "mod", SHA256: sha, Size: size}
	l.b.Plan.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("c", 64), Dir: "go", Blobs: []protocol.CacheBlob{b}}}
	l.b.Node = "node-b"
	l.b.RestoreWait = 5 * time.Second
	return l, st, data, b
}

// runRestore pushes the lease's restore blobs to a node holding copies.
func runRestore(t *testing.T, l *Lease, copies *ReadThrough, w *wire) (map[string]bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Restore(ctx, wireLink{w}) }()
	n := &NodeRestorer{Copies: copies, Feed: wireFeed{w}, Realm: l.b.Plan.Namespace, Wait: 5 * time.Second}
	got := n.Fetch(ctx, l.b.Plan.Restore[0].Blobs)
	return got, <-done
}

func TestRestoreColdThenHeldCopyNeedsNoChunks(t *testing.T) {
	l, _, data, b := coordRestoreFixture(t)
	copies, err := NewReadThrough(t.TempDir(), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	w := newWire()
	if got, err := runRestore(t, l, copies, w); err != nil || !got[b.SHA256] {
		t.Fatalf("cold restore got=%v err=%v", got, err)
	}
	if w.chunks.Load() != 3 {
		t.Fatalf("cold restore sent %d chunks", w.chunks.Load())
	}
	f, err := copies.Open(context.Background(), l.b.Plan.Namespace, b.SHA256, b.Size, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	rows, err := l.h.Ledger.CacheLocality(context.Background(), "node-b", l.b.Plan.Namespace, strings.Repeat("c", 64))
	if err != nil || len(rows) != 1 || rows[0].Bytes != int64(len(data)) {
		t.Fatalf("locality=%+v err=%v", rows, err)
	}
	w2 := newWire()
	if got, err := runRestore(t, l, copies, w2); err != nil || !got[b.SHA256] {
		t.Fatalf("held restore got=%v err=%v", got, err)
	}
	if n := w2.chunks.Load(); n != 0 {
		t.Fatalf("a held copy cost %d chunk frames", n)
	}
}

func TestRestoreCorruptCopyDiscardedAndFetchedAgain(t *testing.T) {
	l, _, _, b := coordRestoreFixture(t)
	copies, _ := NewReadThrough(t.TempDir(), 8<<20)
	if got, err := runRestore(t, l, copies, newWire()); err != nil || !got[b.SHA256] {
		t.Fatalf("seed got=%v err=%v", got, err)
	}
	name, err := copies.name(l.b.Plan.Namespace, b.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(name)
	raw[7] ^= 0xff
	if err = os.WriteFile(name, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	w := newWire()
	if got, err := runRestore(t, l, copies, w); err != nil || !got[b.SHA256] {
		t.Fatalf("corrupt copy got=%v err=%v", got, err)
	}
	if w.chunks.Load() != 3 {
		t.Fatalf("a corrupt copy was served: %d chunk frames", w.chunks.Load())
	}
	f, err := copies.Open(context.Background(), l.b.Plan.Namespace, b.SHA256, b.Size, nil)
	if err != nil {
		t.Fatalf("copy not replaced: %v", err)
	}
	_ = f.Close()
}

// flipOnce changes one byte of the final chunk of the first pass only.
func flipOnce() func(*protocol.Chunk) {
	var done atomic.Bool
	return func(c *protocol.Chunk) {
		if !c.Final || done.Swap(true) {
			return
		}
		raw, _ := base64.StdEncoding.DecodeString(c.Data)
		raw[len(raw)/2] ^= 1
		c.Data = base64.StdEncoding.EncodeToString(raw)
	}
}

func TestRestoreFlippedByteFailsAndResumes(t *testing.T) {
	l, _, _, b := coordRestoreFixture(t)
	copies, _ := NewReadThrough(t.TempDir(), 8<<20)
	w := newWire()
	w.flip = flipOnce()
	if got, err := runRestore(t, l, copies, w); err != nil || !got[b.SHA256] {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if n := w.chunks.Load(); n != 6 {
		t.Fatalf("want two full passes (6 chunks), got %d", n)
	}
	f, err := copies.Open(context.Background(), l.b.Plan.Namespace, b.SHA256, b.Size, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestRestoreAlwaysFlippedLeavesNoCopy(t *testing.T) {
	l, _, _, b := coordRestoreFixture(t)
	copies, _ := NewReadThrough(t.TempDir(), 8<<20)
	w := newWire()
	w.flip = func(c *protocol.Chunk) {
		if c.Final {
			raw, _ := base64.StdEncoding.DecodeString(c.Data)
			raw[0] ^= 1
			c.Data = base64.StdEncoding.EncodeToString(raw)
		}
	}
	got, err := runRestore(t, l, copies, w)
	if got[b.SHA256] || err == nil {
		t.Fatalf("a corrupt transfer was accepted: got=%v err=%v", got, err)
	}
	if _, err = copies.Open(context.Background(), l.b.Plan.Namespace, b.SHA256, b.Size, nil); err == nil {
		t.Fatal("a failed fetch left a copy to bind")
	}
}

func TestRestoreRefusalsAreAudited(t *testing.T) {
	l, s, _, b := coordRestoreFixture(t)
	ctx := context.Background()
	var refused *entries.Refused
	if _, _, err := l.OpenRestore(ctx, strings.Repeat("d", 64)); !errors.As(err, &refused) || refused.Reason != entries.ReasonNotPlanned {
		t.Fatalf("outside the plan: %v", err)
	}
	assertAudit(t, s, entries.ReasonNotPlanned)
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := l.OpenRestore(cctx, b.SHA256); err == nil {
		t.Fatal("a cancelled request was served")
	}
	assertAudit(t, s, "cancelled")
	l.b.Epoch++ // a request from an epoch that is no longer the lease's
	if _, _, err := l.OpenRestore(ctx, b.SHA256); !transfer.IsReason(err, transfer.ReasonStaleEpoch) {
		t.Fatalf("stale epoch: %v", err)
	}
	assertAudit(t, s, transfer.ReasonStaleEpoch)
}
