package s3store

import (
	"context"
	"sync"
	"sync/atomic"
)

// DefaultMaxLargeUploads is the number of uploads above firstChunk bytes that may run at once per Store.
const DefaultMaxLargeUploads = 8

// bufPool recycles the PartSize buffers used by large uploads, so memory is bounded by the slot count and
// small uploads allocate nothing big.
var bufPool = sync.Pool{New: func() any { b := make([]byte, PartSize); return &b }}

// poolGets counts buffers taken from the pool (tests assert that a small upload takes none).
var poolGets atomic.Int64

func getBuf() *[]byte {
	poolGets.Add(1)
	return bufPool.Get().(*[]byte)
}

func putBuf(b *[]byte) { bufPool.Put(b) }

// acquire takes an upload slot, waiting until one frees up or ctx ends. The returned func releases the slot.
// A Store without slots (zero value in tests) is unlimited.
func (s *Store) acquire(ctx context.Context) (func(), error) {
	if s.slots == nil {
		return func() {}, nil
	}
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
