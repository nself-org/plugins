// Package probe observes node capabilities with fixed read-only commands.
package probe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

const cacheTTL = 60 * time.Second

// Prober updates a registered node with a fresh capability snapshot.
type Prober interface {
	Probe(context.Context, string) (registry.Node, error)
}

type commandRunner func(context.Context, string) (string, error)

// Shared limits cover every prober instance in this process.
var slots = make(chan struct{}, 8)
var hosts [256]sync.Mutex

func hostLock(id string) *sync.Mutex {
	var hash uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		hash ^= uint32(id[i])
		hash *= 16777619
	}
	return &hosts[hash%uint32(len(hosts))]
}

func observed[T any](v *T, now time.Time) model.Fact[T] {
	confidence := "unknown"
	if v != nil {
		confidence = "known"
	}
	return model.Fact[T]{Value: v, Source: "probe", ObservedAt: now, Confidence: confidence}
}

func value[T any](v T, now time.Time) model.Fact[T] { return observed(&v, now) }

// run serializes each host, enforces the fleet cap, and uses the registry's
// durable snapshot as the 60-second cache. A failed probe never refreshes it.
func run(ctx context.Context, r *registry.Registry, id string, collect func(context.Context, *model.Capability) error) (registry.Node, error) {
	if r == nil || id == "" {
		return registry.Node{}, fmt.Errorf("probe requires registry and node id")
	}
	mu := hostLock(id)
	mu.Lock()
	defer mu.Unlock()
	n, err := r.Get(ctx, id)
	if err != nil {
		return registry.Node{}, err
	}
	if n.Capability.Verified.By == "probe" && time.Since(n.Capability.Verified.At) < cacheTTL && time.Since(n.Capability.Verified.At) >= 0 {
		return n, nil
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return registry.Node{}, ctx.Err()
	}
	c := n.Capability
	if err := collect(ctx, &c); err != nil {
		return registry.Node{}, err
	}
	c.Verified = model.CapabilityVerified{By: "probe", At: time.Now().UTC()}
	n, _, err = r.UpdateCapability(ctx, id, c, "probe")
	return n, err
}
