package burst

// Purpose: the burst lifecycle engine (Epic D12): intent-first provisioning,
// one job per ephemeral node, revoke at teardown, teardown retried until the
// resource is gone, and a label-bound sweeper. This file holds the Engine, its
// options and the small shared helpers; provision.go, teardown.go, sweep.go and
// lifecycle.go hold the operations.
// Inputs:  Options (store, providers, policy, clock, revoker, exclusion set, alert sink).
// Outputs: an Engine whose methods are safe for concurrent use.
// Constraints: no provider call before the intent row is committed; the engine
// deletes only resources labelled role=ci-ephemeral with its own coordinator id
// and never one in the deploy-host exclusion set; an unreadable exclusion set or
// policy refuses (fail closed); an alert or audit text never carries a provider
// error text (it can echo a credential), only a class.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Config holds the lifetimes of Epic D12. Zero fields take the defaults.
type Config struct {
	// MaxLifetime is the longest a machine may live (default job timeout + 30 min: the caller sets it).
	MaxLifetime time.Duration
	// Grace, SweepEvery and RetryWindow extend the reservation: the machine may outlive
	// MaxLifetime by the grace, the sweep interval and the time a failed teardown is retried.
	Grace, SweepEvery, RetryWindow time.Duration
	// IdleTimeout is how long a joined node may sit without a job (5 min).
	IdleTimeout time.Duration
}

// The defaults.
const (
	DefaultMaxLifetime = 60 * time.Minute
	DefaultGrace       = 10 * time.Minute
	DefaultSweepEvery  = 5 * time.Minute
	DefaultRetryWindow = 30 * time.Minute
	DefaultIdleTimeout = 5 * time.Minute
)

func (c Config) norm() Config {
	for _, f := range []struct {
		v *time.Duration
		d time.Duration
	}{{&c.MaxLifetime, DefaultMaxLifetime}, {&c.Grace, DefaultGrace}, {&c.SweepEvery, DefaultSweepEvery}, {&c.RetryWindow, DefaultRetryWindow}, {&c.IdleTimeout, DefaultIdleTimeout}} {
		if *f.v <= 0 {
			*f.v = f.d
		}
	}
	return c
}

// LifetimeMs is the longest a machine can hold money: max_lifetime + grace + sweep interval + retry window.
func (c Config) LifetimeMs() int64 {
	c = c.norm()
	return (c.MaxLifetime + c.Grace + c.SweepEvery + c.RetryWindow).Milliseconds()
}

// OrphanAfter is the age past which the sweeper forces a deletion: max_lifetime + grace.
func (c Config) OrphanAfter() time.Duration { c = c.norm(); return c.MaxLifetime + c.Grace }

// NodeRevoker revokes an enrolled node (NODE-05 lifecycle.NodeRevoke with NODE-07's
// deny-list hook). It must be idempotent: a node that is already revoked or gone is success.
type NodeRevoker interface {
	Revoke(ctx context.Context, nodeID string) error
}

// Alert is the event the operator is told once per failed teardown or orphan.
type Alert struct {
	Kind, Provider, Intent, Resource, Detail string
}

// The alert kinds.
const (
	AlertTeardownFailed = "burst.teardown_failed"
	AlertOrphan         = "burst.orphan"
)

// ErrKilled is returned by a Fault hook to stop the engine at a point exactly as a
// crash would: nothing after the point runs and nothing is cleaned up.
var ErrKilled = errors.New("burst: coordinator killed (fault injection)")

// The fault points (Epic D12 acceptance).
const (
	PointAfterIntent = "after-intent"
	PointAfterCreate = "after-create"
	PointAfterJoin   = "after-join"
	PointMidJob      = "mid-job"
	PointMidTeardown = "mid-teardown"
)

// Options builds an Engine.
type Options struct {
	Store       *store.Store
	Providers   []Provider
	Coordinator string
	Config      Config
	// Spend resolves the effective spend policy (operator scope); required.
	Spend func(context.Context) (economics.Policy, error)
	// Team reports a TEAM coordinator; cloud providers refuse otherwise (burst.team_only).
	Team func() bool
	// Exclusion reads the deploy-host exclusion set (NODE-14); required, and an error stops every deletion.
	Exclusion func(context.Context) (deployguard.Set, error)
	// Revoker revokes a node at teardown; required once a node can join.
	Revoker NodeRevoker
	// Now is the clock (default time.Now). Alert receives events (may be nil).
	Now   func() time.Time
	Alert func(Alert)
	// NewToken makes the 26-character intent id (default: a ULID from crypto/rand).
	NewToken func(time.Time) (string, error)
	// Fault is a test seam: it is called at each fault point and may return ErrKilled.
	Fault func(point string) error
}

// Engine runs the burst lifecycle of one coordinator.
type Engine struct {
	o         Options
	cfg       Config
	providers map[string]Provider
	mu        sync.Mutex
	orphans   map[string]bool
	loop      *loopState
}

// New validates the options and builds an Engine.
func New(o Options) (*Engine, error) {
	switch {
	case o.Store == nil:
		return nil, errors.New("burst: a store is required")
	case o.Coordinator == "":
		return nil, errors.New("burst: a coordinator id is required")
	case o.Spend == nil:
		return nil, errors.New("burst: a spend policy source is required")
	case o.Exclusion == nil:
		return nil, errors.New("burst: an exclusion set source is required")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Team == nil {
		o.Team = func() bool { return false }
	}
	if o.NewToken == nil {
		o.NewToken = newULID
	}
	e := &Engine{o: o, cfg: o.Config.norm(), providers: map[string]Provider{}, orphans: map[string]bool{}}
	for _, p := range o.Providers {
		if p == nil || p.ID() == "" {
			return nil, errors.New("burst: a provider needs an id")
		}
		if _, dup := e.providers[p.ID()]; dup {
			return nil, fmt.Errorf("burst: provider %q is registered twice", p.ID())
		}
		e.providers[p.ID()] = p
	}
	return e, nil
}

func (e *Engine) now() time.Time { return e.o.Now() }

// at calls the fault hook at a point.
func (e *Engine) at(point string) error {
	if e.o.Fault == nil {
		return nil
	}
	return e.o.Fault(point)
}

// econCtx binds the economics environment the reservation calls read.
func (e *Engine) econCtx(ctx context.Context) context.Context {
	return economics.WithEnv(ctx, economics.Env{Store: e.o.Store, Spend: e.o.Spend, Now: e.now, Coordinator: e.o.Coordinator})
}

func (e *Engine) providerIDs() []string {
	ids := make([]string, 0, len(e.providers))
	for id := range e.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// errClass names a failure without its text: provider and OS errors can echo a credential.
func errClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, ErrUnavailable):
		return "provider unavailable"
	case errors.Is(err, ErrQuota):
		return "provider quota"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	case errors.Is(err, errExcluded):
		return "resource is in the deploy-host exclusion set"
	case errors.Is(err, errStillListed):
		return "resource still listed after teardown"
	}
	return "provider error"
}

var (
	errExcluded    = errors.New("burst: resource is in the exclusion set")
	errStillListed = errors.New("burst: resource still listed")
)
