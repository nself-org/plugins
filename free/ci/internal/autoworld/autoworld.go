// Package autoworld is the one world contributor for sched.World.Exclusions.
//
// Purpose: world.Register allows one owner per World field. Pools (P7-AUTO-03),
// readiness (P7-AUTO-08) and the record-pool rule (P7-AUTO-15) each need to
// keep runners out of placement with a reason, so they register an exclusion
// provider here instead of sharing a file or a field.
// Inputs:  providers added with RegisterExclusion from init() or startup.
// Outputs: World.Exclusions, merged per runner id in provider-name order.
// Constraints: a provider reads the world and returns reasons; it never edits
// it. Every returned code must be in the closed model.PlacementReason set. A
// provider error fails the world build (fail closed). With no provider the
// field stays empty and every placement is byte-identical to before.
package autoworld

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// ExclusionFn returns extra ineligibility reasons per runner id for this world.
type ExclusionFn func(ctx context.Context, w sched.World) (map[string][]model.PlacementReasonDetail, error)

// Name is the contributor name registered with the world builder.
const Name = "autoworld"

var providers = struct {
	sync.RWMutex
	byName map[string]ExclusionFn
}{byName: map[string]ExclusionFn{}}

func init() {
	if err := world.Register(world.Contributor{Name: Name, Field: "Exclusions", Fn: contribute}); err != nil {
		panic("autoworld: " + err.Error())
	}
}

// RegisterExclusion adds an exclusion provider. Names are unique.
func RegisterExclusion(name string, fn ExclusionFn) error {
	if name == "" || fn == nil {
		return fmt.Errorf("autoworld: exclusion provider needs a name and a function")
	}
	providers.Lock()
	defer providers.Unlock()
	if _, dup := providers.byName[name]; dup {
		return fmt.Errorf("autoworld: exclusion provider %q already registered", name)
	}
	providers.byName[name] = fn
	return nil
}

// Providers lists the registered provider names in the order they run.
func Providers() []string {
	providers.RLock()
	defer providers.RUnlock()
	names := make([]string, 0, len(providers.byName))
	for n := range providers.byName {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func contribute(ctx context.Context, w *sched.World) error {
	names := Providers()
	if len(names) == 0 {
		return nil
	}
	providers.RLock()
	fns := make([]ExclusionFn, len(names))
	for i, n := range names {
		fns[i] = providers.byName[n]
	}
	providers.RUnlock()
	merged := map[string][]model.PlacementReasonDetail{}
	for i, fn := range fns {
		got, err := fn(ctx, *w)
		if err != nil {
			return fmt.Errorf("exclusion provider %s: %w", names[i], err)
		}
		runners := make([]string, 0, len(got))
		for id := range got {
			runners = append(runners, id)
		}
		sort.Strings(runners)
		for _, id := range runners {
			for _, d := range got[id] {
				if !model.PlacementReason(d.Code).Valid() {
					return fmt.Errorf("exclusion provider %s: unknown reason code %q", names[i], d.Code)
				}
				merged[id] = append(merged[id], d)
			}
		}
	}
	if len(merged) > 0 {
		w.Exclusions = merged
	}
	return nil
}
