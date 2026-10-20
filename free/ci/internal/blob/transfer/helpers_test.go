package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

func newStore(t *testing.T) *cas.Store {
	t.Helper()
	s, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newReceiver(t *testing.T, s *cas.Store, lease string) *Receiver {
	return &Receiver{Store: s, Realm: cas.ArtifactRealm("proj"), Lease: lease,
		Limits: Limits{MaxFile: 1 << 30}, Live: func(context.Context) error { return nil }}
}

func blobOf(data []byte) Blob {
	return Blob{Stream: StreamArtifact, SHA256: sum(data), Size: int64(len(data)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }}
}

func held(t *testing.T, s *cas.Store, data []byte) bool {
	t.Helper()
	rc, err := s.Open(cas.ArtifactRealm("proj"), sum(data))
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	got, _ := io.ReadAll(rc)
	return bytes.Equal(got, data)
}

// wire plays the coordinator driver between a Sender and a Receiver: it answers digest with
// offset, checks chunk order like the real drivers and commits on the final chunk.
type wire struct {
	t       *testing.T
	r       *Receiver
	offsets chan protocol.Offset

	mu     sync.Mutex
	w      *Writer
	next   int64
	head   transport.BlobHeader
	tamper func(c *protocol.Chunk) // mutates a chunk in flight
	failed error                   // the first error a driver would turn into a dead lease
	digest int
	chunks int
}

func newWire(t *testing.T, r *Receiver) *wire {
	return &wire{t: t, r: r, offsets: make(chan protocol.Offset, 64)}
}

func (w *wire) Digest(d protocol.Digest) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.digest++
	w.head = transport.BlobHeader{Stream: d.Stream, SHA256: d.SHA256, Size: d.Size}
	wr, err := w.r.Open(context.Background(), w.head)
	if err != nil {
		w.failed = err
		return nil
	}
	w.w, w.next = wr, wr.Offset()
	if w.next == w.head.Size {
		if err := wr.Close(); err != nil {
			w.failed = err
		}
	}
	w.offsets <- protocol.Offset{Stream: d.Stream, SHA256: d.SHA256, Offset: w.next}
	return nil
}

func (w *wire) Chunk(c protocol.Chunk) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chunks++
	if w.w == nil || w.failed != nil {
		return nil
	}
	if w.tamper != nil {
		w.tamper(&c)
	}
	if c.Offset != w.next {
		w.offsets <- protocol.Offset{Stream: c.Stream, SHA256: w.head.SHA256, Offset: w.next}
		return nil
	}
	data, err := base64.StdEncoding.Strict().DecodeString(c.Data)
	if err != nil {
		w.failed = err
		return nil
	}
	if _, err = w.w.Write(data); err != nil {
		w.failed = err
		return nil
	}
	w.next += int64(len(data))
	if c.Final {
		if err = w.w.Close(); err != nil {
			w.failed = err
			w.w.offset = 0
		}
	}
	return nil
}

func (w *wire) sender() *Sender { return &Sender{Up: w, Offsets: w.offsets} }
