package isolation

// Purpose: one registry keeps every isolation level behind one interface.
// Inputs:  Register from a profile's own run hook file; Select(level) per job.
// Outputs: the lowest available profile that delivers at least the level.
// Constraints: Select never lowers a level and never returns a process isolator:
// no profile available means E678 and the job waits (Epic D7).

import (
	"context"
	"sort"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Registry holds the registered profiles.
type Registry struct {
	mu       sync.RWMutex
	profiles []Profile
}

// Default is the registry the run hooks fill from init().
var Default = &Registry{}

// Register adds a profile; a second profile of the same declared level replaces the first.
func Register(p Profile) { Default.Register(p) }

// Select picks a profile from the default registry.
func Select(ctx context.Context, level model.Isolation) (Profile, Availability, error) {
	return Default.Select(ctx, level)
}

// Register adds a profile to r.
func (r *Registry) Register(p Profile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, have := range r.profiles {
		if have.Level() == p.Level() {
			r.profiles[i] = p
			return
		}
	}
	r.profiles = append(r.profiles, p)
}

// Profiles returns the registered profiles ordered by declared level.
func (r *Registry) Profiles() []Profile {
	r.mu.RLock()
	out := append([]Profile(nil), r.profiles...)
	r.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		return trust.RankIsolation(out[i].Level()) < trust.RankIsolation(out[j].Level())
	})
	return out
}

// Select returns the lowest-level available profile whose effective level is at
// least the one asked for, with its availability. A request at or below process
// returns a nil Profile and no error: process isolation is the executor baseline.
// Otherwise an unavailable level returns E678 carrying the best reason found.
func (r *Registry) Select(ctx context.Context, level model.Isolation) (Profile, Availability, error) {
	need := trust.RankIsolation(level)
	if level == "" {
		need = trust.RankIsolation("process")
	}
	if need < 0 {
		return nil, Availability{}, Unavailable(ReasonUnknownLevel, "unknown isolation level %q", level)
	}
	if need <= trust.RankIsolation("process") {
		return nil, Availability{Level: "process"}, nil
	}
	reason := ReasonNoProfile
	for _, p := range r.Profiles() {
		if trust.RankIsolation(p.Level()) < need {
			continue
		}
		av := p.Probe(ctx)
		av.Level = Effective(p.Level(), av.Level)
		if av.Available() && trust.RankIsolation(av.Level) >= need {
			return p, av, nil
		}
		if av.Reason != "" {
			reason = av.Reason
		}
	}
	return nil, Availability{Reason: reason}, Unavailable(reason, "isolation level %q is unavailable on this runner", level)
}

// List asks every Lister profile for the resources carrying label.
func (r *Registry) List(ctx context.Context, label string) ([]string, error) {
	var out []string
	for _, p := range r.Profiles() {
		if l, ok := p.(Lister); ok {
			names, err := l.List(ctx, label)
			if err != nil {
				return out, err
			}
			out = append(out, names...)
		}
	}
	return out, nil
}
