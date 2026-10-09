package lease

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Config controls every lease timer and permits deterministic clock injection.
type Config struct {
	Clock        func() time.Time
	Heartbeat    time.Duration
	TTL          time.Duration
	Intermittent time.Duration
	BreakerK     int
	HostedPoll   time.Duration
}

func (c Config) defaults() Config {
	if c.Clock == nil {
		c.Clock = time.Now
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = 10 * time.Second
	}
	if c.TTL <= 0 {
		c.TTL = 45 * time.Second
	}
	if c.Intermittent <= 0 {
		c.Intermittent = 120 * time.Second
	}
	if c.BreakerK <= 0 {
		c.BreakerK = 3
	}
	if c.HostedPoll <= 0 {
		c.HostedPoll = 30 * time.Second
	}
	return c
}

// Engine coordinates lease operations; all authoritative state stays in Store.
type Engine struct {
	Store    *store.Store
	Nodes    *registry.Registry
	Config   Config
	mu       sync.Mutex
	last     map[string]time.Time
	profiles map[string]LeaseProfile
	gone     map[string]time.Time
	polled   map[string]time.Time
	stale    atomic.Int64
}

func New(s *store.Store, nodes *registry.Registry, config Config) *Engine {
	return &Engine{Store: s, Nodes: nodes, Config: config.defaults(), last: make(map[string]time.Time), profiles: make(map[string]LeaseProfile), gone: make(map[string]time.Time), polled: make(map[string]time.Time)}
}

// Grant issues one fenced lease after the store's atomic capacity and state checks.
func (e *Engine) Grant(ctx context.Context, host string, d store.Demand, incarnation int64) (int64, error) {
	if d.TTL <= 0 {
		d.TTL = e.Config.TTL
	}
	return e.Store.LeaseGrant(ctx, host, d, incarnation)
}

// Renew validates every heartbeat and durably extends the deadline every TTL/3.
func (e *Engine) Renew(ctx context.Context, leaseID string, epoch int64, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = e.Config.TTL
	}
	now := e.Config.Clock()
	e.mu.Lock()
	defer e.mu.Unlock()
	last := e.last[leaseID]
	durable := last.IsZero() || now.Sub(last) >= ttl/3
	if err := e.Store.LeaseRenew(ctx, leaseID, epoch, now, ttl, durable); err != nil {
		return e.countStale(err)
	}
	if durable {
		e.last[leaseID] = now
	}
	return nil
}

// Start gives leases of older coordinator incarnations a full TTL to reconnect.
func (e *Engine) Start(ctx context.Context, coordinatorID string, incarnation int64) (int64, error) {
	return e.Store.LeaseGrace(ctx, coordinatorID, incarnation, e.Config.Clock(), e.Config.TTL)
}

// Sweep marks expired remote leases lost through the store transition API.
func (e *Engine) Sweep(ctx context.Context) (int, error) {
	now := e.Config.Clock()
	leases, err := e.Store.LeaseExpired(ctx, now)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, l := range leases {
		if err := e.lose(ctx, l, "lease.expired", true); err != nil {
			var storeErr *store.Error
			if errors.As(err, &storeErr) && storeErr.Code == "E605" {
				continue
			}
			return count, err
		}
		count++
	}
	return count, nil
}

func (e *Engine) lose(ctx context.Context, l store.RemoteLease, reason string, expired bool) error {
	return e.Store.LeaseFail(ctx, l, e.Config.Clock(), reason, expired)
}

// Reconnect applies the agent's authoritative self-fence report after restart grace.
func (e *Engine) Reconnect(ctx context.Context, leaseID string, epoch int64, status string) error {
	l, err := e.Store.LeaseGet(ctx, leaseID)
	if err != nil {
		return e.countStale(err)
	}
	if l.Epoch != epoch {
		return e.countStale(&store.Error{Code: "E662", Message: "stale lease epoch; stop the job and clean its workspace"})
	}
	switch status {
	case "running":
		return e.Renew(ctx, leaseID, epoch, 0)
	case "self_fenced":
		return e.lose(ctx, l, "agent.self_fenced", false)
	default:
		return fmt.Errorf("invalid reconnect status %q", status)
	}
}

// Result accepts a carrier result only for the current epoch and state.
func (e *Engine) Result(ctx context.Context, leaseID string, epoch int64) error {
	return e.countStale(e.Store.LeaseResult(ctx, leaseID, epoch))
}

// Fence validates a log or other carrier action against the durable epoch.
func (e *Engine) Fence(ctx context.Context, leaseID string, epoch int64) error {
	return e.countStale(e.Store.LeaseFence(ctx, leaseID, epoch))
}

func (e *Engine) countStale(err error) error {
	var coded *store.Error
	if errors.As(err, &coded) && coded.Code == "E662" {
		e.stale.Add(1)
	}
	return err
}

// StaleCount is the number of rejected stale carrier operations in this process.
func (e *Engine) StaleCount() int64 { return e.stale.Load() }

// Breaker records infra streaks and changes node state once at the threshold.
func (e *Engine) Breaker(ctx context.Context, nodeID string, infra bool) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	count, trip, err := e.Store.LeaseFailure(ctx, nodeID, infra, e.Config.BreakerK)
	if err != nil || !trip {
		return count, err
	}
	if e.Nodes == nil {
		return count, fmt.Errorf("node registry required for breaker")
	}
	node, err := e.Nodes.Get(ctx, nodeID)
	if err != nil {
		return count, err
	}
	if node.Record.State == "maintenance" {
		return count, e.Store.LeaseBreakerTripped(ctx, nodeID)
	}
	_, err = e.Nodes.SetState(ctx, nodeID, "maintenance", "three consecutive infra failures", "coordinator")
	if err != nil {
		return count, err
	}
	return count, e.Store.LeaseBreakerTripped(ctx, nodeID)
}
