package cas

// Purpose: stream a blob to a consumer in one read, verifying its digest as the
// consumer reads (D-1008 round 3). Open copies the whole blob into an unlinked
// snapshot and hashes it before serving a byte: for a 100 MB build cache that is
// a second 100 MB write and about 200 ms on the start path of a job.
// Inputs:  a realm, a digest and a consumer of the bytes.
// Outputs: nil when the consumer succeeded and the bytes it read are the blob.
// Constraints: the consumer reads the one descriptor the store checked, so what
// is hashed is exactly what it consumed (no second read, no window to swap the
// file); it must treat the bytes as untrusted until ReadVerified returns nil and
// throw away what it built on any error. A digest or size mismatch quarantines
// the blob, like Open, and is reported even when the consumer failed first (a
// corrupt blob is named as corrupt, not as an unreadable archive).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"sync"
)

// ReadVerified calls fn with the bytes of the named blob and verifies them.
func (s *Store) ReadVerified(realm Realm, digest string, fn func(io.Reader) error) error {
	return s.ReadVerifiedContext(context.Background(), realm, digest, fn)
}

// ReadVerifiedContext bounds consumption and the remaining verification drain.
// Cancellation leaves the blob in place because an incomplete hash proves no corruption.
func (s *Store) ReadVerifiedContext(ctx context.Context, realm Realm, digest string, fn func(io.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, info, path, err := s.openBlob(realm, digest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := newAsyncHash()
	count := &countingReader{r: &verifiedContextReader{ctx: ctx, r: f}, h: h}
	if s.beforeHash != nil {
		s.beforeHash()
	}
	fnErr := fn(count)
	_, drainErr := io.Copy(io.Discard, count) // the rest of the blob, so the digest covers all of it
	sum := h.sum()                            // always: the hashing goroutine ends here
	if drainErr != nil {
		return drainErr
	}
	if count.n != info.Size() || hex.EncodeToString(sum) != digest {
		mismatch := ErrDigestMismatch
		if count.n != info.Size() {
			mismatch = ErrSizeMismatch
		}
		if err := s.locked(func() error { return s.quarantine(path, info) }); err != nil {
			return err
		}
		return mismatch
	}
	return fnErr
}

// countingReader counts what the consumer reads and hands each chunk to the hasher.
type countingReader struct {
	r io.Reader
	n int64
	h *asyncHash
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.h.write(p[:n])
	return n, err
}

// asyncHash is a SHA-256 computed on its own goroutine, so hashing a 100 MB
// build cache overlaps with the consumer unpacking it instead of adding to it
// (D-1008). Chunks are copied (the consumer reuses its buffer) and queued.
type asyncHash struct {
	h    hash.Hash
	ch   chan *[]byte
	done chan struct{}
	pool sync.Pool
}

func newAsyncHash() *asyncHash {
	a := &asyncHash{h: sha256.New(), ch: make(chan *[]byte, 16), done: make(chan struct{})}
	go func() {
		defer close(a.done)
		for b := range a.ch {
			a.h.Write(*b)
			*b = (*b)[:0]
			a.pool.Put(b)
		}
	}()
	return a
}

func (a *asyncHash) write(p []byte) {
	if len(p) == 0 {
		return
	}
	b, _ := a.pool.Get().(*[]byte)
	if b == nil {
		b = new([]byte)
	}
	*b = append((*b)[:0], p...)
	a.ch <- b
}

// sum waits for the queued chunks and returns the digest; call it once.
func (a *asyncHash) sum() []byte {
	close(a.ch)
	<-a.done
	return a.h.Sum(nil)
}

type verifiedContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *verifiedContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
