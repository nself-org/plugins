package s3store

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxUploads is the number of uploads (of any size) that may run at once per Store.
const DefaultMaxUploads = 64

// DefaultSlotWait is how long an upload waits for a free slot before failing with ErrBusy.
const DefaultSlotWait = 30 * time.Second

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

// acquire takes a slot from sem, waiting until one frees up, ctx ends, or the Store's slot wait (30s by default)
// elapses (ErrBusy). The returned func releases the slot. A nil sem (zero Store in tests) is unlimited.
func (s *Store) acquire(ctx context.Context, sem chan struct{}) (func(), error) {
	if sem == nil {
		return func() {}, nil
	}
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	default:
	}
	wait := s.slotWait
	if wait <= 0 {
		wait = DefaultSlotWait
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.C:
		return nil, ErrBusy
	}
}
