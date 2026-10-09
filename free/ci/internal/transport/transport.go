package transport

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Driver delivers a lease over one carrier.
type Driver interface {
	Kind() string
	Run(context.Context, model.Capability, protocol.Lease, Sink) error
}
type BlobHeader struct {
	Stream string
	SHA256 string
	Size   int64
}
type BlobWriter interface {
	io.WriteCloser
	Offset() int64
}

// Sink consumes agent output without retaining credentials in a store row.
type Sink interface {
	Log(context.Context, protocol.Log) error
	ArtifactRef(context.Context, protocol.ArtifactRef) error
	Result(context.Context, protocol.Result) error
	Blob(context.Context, BlobHeader) (BlobWriter, error)
	CacheRef(context.Context, protocol.CacheRef) error
}
type Preparer func(context.Context, model.Capability, *protocol.Lease, model.Job) error

var mu sync.RWMutex
var drivers = map[string]Driver{}
var preparers []Preparer

// Register rejects duplicate or empty driver kinds without replacing one.
func Register(d Driver) error {
	if d == nil || d.Kind() == "" {
		return fmt.Errorf("invalid transport driver")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := drivers[d.Kind()]; ok {
		return fmt.Errorf("duplicate transport driver: %s", d.Kind())
	}
	drivers[d.Kind()] = d
	return nil
}
func Lookup(kind string) (Driver, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := drivers[kind]
	return d, ok
}
func RegisterPreparer(p Preparer) error {
	if p == nil {
		return fmt.Errorf("nil preparer")
	}
	mu.Lock()
	defer mu.Unlock()
	preparers = append(preparers, p)
	return nil
}

// Deliver prepares a lease in registration order, then invokes its driver.
func Deliver(ctx context.Context, node model.Capability, lease protocol.Lease, sink Sink) error {
	mu.RLock()
	steps := append([]Preparer(nil), preparers...)
	mu.RUnlock()
	for _, p := range steps {
		if err := p(ctx, node, &lease, lease.Job); err != nil {
			return err
		}
	}
	d, ok := Lookup(node.Identity.Transport)
	if !ok {
		return fmt.Errorf("unknown transport: %s", node.Identity.Transport)
	}
	return d.Run(ctx, node, lease, sink)
}
