package runtime

// Purpose: the world contributor "providers" (Epic D4, SCHED-18). It owns sched.World.Hosted: one
// hosted runner for every capability a registered provider's Describe reports for each target
// (provider, pipeline repo), with the provider's health applied. The scheduler then places jobs on
// hosted runners through the same admission path as every other runner.
// Inputs:  the registered providers, the Targets source (the queries to Describe), the optional store
//          for runner reservations, providers.HealthOf.
// Outputs: w.Hosted sorted by runner id (the world digest is stable for the same Describe output);
//          Skipped() lists the providers that contributed nothing and why.
// Constraints: only the Hosted field changes. A provider whose credential does not resolve (E700) or
//          whose Describe fails contributes no runner and is listed in Skipped, so one provider's
//          outage never fails the whole world. Identity.Provider, Identity.Transport ("provider") and
//          Location.Kind ("hosted") are set here, never taken from the provider. A provider that
//          reports down keeps its runners in the world, offline and ProviderDown, so the placement
//          explanation says provider.down instead of hiding the runner. Dormant unless compat.V15().
//          A capability whose id equals the id of an enrolled node, the local machine or another hosted
//          runner is dropped and listed (E708): the dispatcher resolves a placement by runner id,
//          and a hosted job must never be admitted onto a machine of the team's.
//          Skipped() is the explanation source for a provider that is absent from the world: a runner
//          that does not exist has no placement reason to carry the code, so `ci providers show` and
//          `ci doctor --providers` (P7-HOST-12) and `plan --explain` read the E700 line from here.

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// Skip says why a provider contributed no runner in the last pass.
type Skip struct{ Provider, Code, Reason string }

// Contributor fills sched.World.Hosted from the registered providers.
type Contributor struct {
	mu      sync.Mutex
	targets func(context.Context) ([]providers.Query, error)
	// targetsFor, when set, names the queries per provider id and replaces targets (a provider that
	// gets none is not described): a repository path of one forge is never offered to another.
	targetsFor func(ctx context.Context, provider string) ([]providers.Query, error)
	store      *store.Store
	skipped    []Skip
	creds      map[string]credCheck
	// describeTTL > 0 keeps a Describe result per provider and query for that long: the world is built
	// every dispatcher tick and Describe may cost an API request.
	describeTTL time.Duration
	described   map[describeKey]describeHit
}

type describeKey struct {
	provider string
	q        providers.Query
}

type describeHit struct {
	caps []model.Capability
	at   time.Time
}

// credCheck remembers whether a provider's credential resolved, for credCheckTTL: a world is built
// every dispatcher tick, and an unresolved GitHub login would otherwise run `gh auth token` each time.
type credCheck struct {
	err error
	at  time.Time
}

const credCheckTTL = 30 * time.Second

var defaultContributor = &Contributor{}

func init() {
	if err := world.Register(world.Contributor{Name: "providers", Field: "Hosted", Fn: defaultContributor.Fill}); err != nil {
		panic(err)
	}
}

// DefaultContributor returns the contributor registered with the world builder.
func DefaultContributor() *Contributor { return defaultContributor }

// SetTargets installs the source of the queries to Describe (the project, repo and visibility of the
// pipelines being placed). With none, each provider is described once with an empty query.
func (c *Contributor) SetTargets(fn func(context.Context) ([]providers.Query, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targets = fn
}

// SetTargetsFor installs a per-provider source of queries; it replaces SetTargets. nil removes it.
func (c *Contributor) SetTargetsFor(fn func(ctx context.Context, provider string) ([]providers.Query, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targetsFor = fn
}

// SetDescribeTTL keeps each Describe result for d (0 = ask every pass). Failures are never kept.
func (c *Contributor) SetDescribeTTL(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.describeTTL, c.described = d, nil
}

// describe returns the capabilities of p for q, from the cache when it is fresh.
func (c *Contributor) describe(ctx context.Context, p providers.Provider, q providers.Query) ([]model.Capability, error) {
	key := describeKey{p.ID(), q}
	c.mu.Lock()
	ttl := c.describeTTL
	hit, ok := c.described[key]
	c.mu.Unlock()
	if ttl > 0 && ok && time.Since(hit.at) < ttl {
		return hit.caps, nil
	}
	caps, err := p.Describe(ctx, q)
	if err != nil || ttl <= 0 {
		return caps, err
	}
	c.mu.Lock()
	if c.described == nil {
		c.described = map[describeKey]describeHit{}
	}
	c.described[key] = describeHit{caps: caps, at: time.Now()}
	c.mu.Unlock()
	return caps, nil
}

// ForgetCredentials drops the cached credential checks (after a policy change).
func (c *Contributor) ForgetCredentials() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.creds = nil
}

// SetStore lets Fill take the slots and capacity already reserved on a hosted runner from the store.
func (c *Contributor) SetStore(st *store.Store) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = st
}

// Skipped returns the providers that contributed nothing in the last pass.
func (c *Contributor) Skipped() []Skip {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Skip(nil), c.skipped...)
}

// Fill sets w.Hosted. It is the world.Contributor function.
func (c *Contributor) Fill(ctx context.Context, w *sched.World) error {
	// compat.V15(P7-HOST-16): no hosted runners in the world -> hosted runners from registered providers
	if !compat.V15() {
		return nil
	}
	c.mu.Lock()
	targets, targetsFor, st := c.targets, c.targetsFor, c.store
	c.mu.Unlock()
	queries := []providers.Query{{}}
	if targets != nil {
		var err error
		if queries, err = targets(ctx); err != nil {
			return err
		}
	}
	var reserved map[string]store.SchedReservation
	if st != nil {
		snap, err := st.WorldSnapshot(ctx)
		if err != nil {
			return err
		}
		reserved = snap.Reservations
	}
	var runners []sched.Runner
	var skipped []Skip
	seen := map[string]bool{}
	for _, r := range w.Runners { // an enrolled node or the local machine owns its id: a hosted capability may not take it over
		seen[r.ID] = true
	}
	for _, p := range providers.All() {
		if _, ok := p.(providers.Dispatcher); !ok {
			continue
		}
		qs := queries
		if targetsFor != nil {
			var err error
			if qs, err = targetsFor(ctx, p.ID()); err != nil {
				return err
			}
			if len(qs) == 0 { // not a target of this world: neither described nor listed as skipped
				continue
			}
		}
		if err := c.credential(ctx, p.ID()); err != nil {
			skipped = append(skipped, skip(p.ID(), "E700", err))
			continue
		}
		health := providers.HealthOf(ctx, p.ID())
		for _, q := range qs {
			caps, err := c.describe(ctx, p, q)
			if err != nil {
				skipped = append(skipped, skip(p.ID(), "E704", err))
				continue
			}
			for _, cp := range caps {
				if cp.Identity.ID == "" {
					continue
				}
				if seen[cp.Identity.ID] {
					skipped = append(skipped, Skip{Provider: p.ID(), Code: "E708", Reason: "runner id " + cp.Identity.ID + " is already taken"})
					continue
				}
				seen[cp.Identity.ID] = true
				runners = append(runners, hostedRunner(cp, p.ID(), health, reserved[cp.Identity.ID]))
			}
		}
	}
	sort.Slice(runners, func(i, j int) bool { return runners[i].ID < runners[j].ID })
	w.Hosted = runners
	c.mu.Lock()
	c.skipped = skipped
	c.mu.Unlock()
	return nil
}

func (c *Contributor) credential(ctx context.Context, provider string) error {
	c.mu.Lock()
	hit, ok := c.creds[provider]
	c.mu.Unlock()
	if ok && time.Since(hit.at) < credCheckTTL {
		return hit.err
	}
	_, err := Credential(ctx, provider)(ctx)
	c.mu.Lock()
	if c.creds == nil {
		c.creds = map[string]credCheck{}
	}
	c.creds[provider] = credCheck{err: err, at: time.Now()}
	c.mu.Unlock()
	return err
}

func skip(provider, fallback string, err error) Skip {
	var pe *providers.Error
	if errors.As(err, &pe) && pe.Code != "" {
		return Skip{Provider: provider, Code: pe.Code, Reason: pe.Reason}
	}
	return Skip{Provider: provider, Code: fallback, Reason: "provider unavailable"}
}

func assigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Confidence: "known", Source: "assigned", ObservedAt: time.Time{}}
}

func hostedRunner(c model.Capability, provider string, health providers.Health, r store.SchedReservation) sched.Runner {
	c.Identity.Provider, c.Identity.Transport = provider, Kind
	c.Location.Kind = assigned("hosted")
	down := health.State == "down"
	switch {
	case down:
		c.Availability.State = assigned("offline")
	case c.Availability.State.Value == nil:
		c.Availability.State = assigned("online")
	}
	run := sched.Runner{ID: c.Identity.ID, Capability: c, Decl: sched.DeclarationFromCapability(c, false, ""), Provider: provider, Location: "hosted", ProviderDown: down}
	run.Decl.Hosted = true
	if v := c.Economics.Class.Value; v != nil {
		run.Class = *v
	}
	run.Active = *c.Availability.State.Value == "online"
	if v := c.Availability.Slots.Value; v != nil {
		run.Avail.Slots = max(0, *v-int(r.Slots))
	}
	if v := c.Resources.CPU.Value; v != nil {
		run.Avail.CPU = max(0, int64(*v)-r.CPU)
	}
	if v := c.Resources.MemMB.Value; v != nil {
		run.Avail.MemMB = max(0, *v-r.MemMB)
	}
	return run
}
