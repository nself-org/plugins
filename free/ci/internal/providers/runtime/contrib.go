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
	store   *store.Store
	skipped []Skip
}

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
	targets, st := c.targets, c.store
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
	for _, p := range providers.All() {
		if _, ok := p.(providers.Dispatcher); !ok {
			continue
		}
		if _, err := Credential(ctx, p.ID())(ctx); err != nil {
			skipped = append(skipped, skip(p.ID(), "E700", err))
			continue
		}
		health := providers.HealthOf(ctx, p.ID())
		for _, q := range queries {
			caps, err := p.Describe(ctx, q)
			if err != nil {
				skipped = append(skipped, skip(p.ID(), "E704", err))
				continue
			}
			for _, cp := range caps {
				if cp.Identity.ID == "" || seen[cp.Identity.ID] {
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
