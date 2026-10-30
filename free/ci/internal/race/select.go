package race

// Purpose: choose the runners of one race. Each sibling is placed by
// sched.Place on a world that excludes every runner already chosen, so no two
// siblings share a runner and every choice passes the scheduler's own
// eligibility (capacity, limits, trust admission, overrides).
// Inputs:  the placement world, the job's placement request and the width.
// Outputs: a Choice: the runners in placement order and why others were left out.
// Constraints: pure; the caller's world is never mutated. A release-class job
// is never raced (CACHE-05 at-most-once). Hosted, paid and ephemeral targets
// (any class but local and owned) and runners that fail trust admission are
// excluded before placement. The width never exceeds the free slots of the
// runners left, so a race cannot starve other work.

import (
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Reasons a runner or a job is kept out of a race.
const (
	ReasonRelease  = "race.release"   // a release-class or idempotent:false job runs at most once
	ReasonSpend    = "race.spend"     // a hosted, paid or ephemeral target
	ReasonTrust    = "race.untrusted" // the runner fails trust admission for the job
	ReasonSibling  = "race.sibling"   // a sibling already holds the runner
	ReasonNoTarget = "race.no-target" // fewer than two distinct eligible runners
)

// Choice is the outcome of Select.
type Choice struct {
	Runners  []string          // distinct runner ids, in placement order
	Excluded map[string]string // runner id -> reason it cannot take a sibling
	Reason   string            // why the job is not raced ("" when len(Runners) >= 2)
}

// Raceable says the choice has at least two siblings.
func (c Choice) Raceable() bool { return len(c.Runners) >= 2 }

// freeClasses are the cost classes a sibling may run on: nothing is spent.
var freeClasses = []string{"local", "owned"}

// Spend says a runner is a target a race never uses: a hosted, paid or ephemeral class.
func Spend(w sched.World, r sched.Runner) bool {
	class := r.Class
	if c := w.Costs[r.ID].Class; c != "" { // the priced class wins over the claimed one
		class = c
	}
	return r.Ephemeral || r.Location == "hosted" || !slices.Contains(freeClasses, class)
}

// Trusted says the runner passes trust admission for the job.
func Trusted(w sched.World, j sched.Job, r sched.Runner) bool {
	jf := trust.JobFacts{Trust: j.Trust, SecretClasses: j.SecretClasses, Network: j.Network, PersonalOwnRun: j.PersonalOwnRun, Release: j.Pipeline.Release}
	return len(trust.Admissible(jf, r.Decl, w.Policy.Trust)) == 0
}

// Select picks up to width distinct runners for j. release marks a
// release-class job (model.Job.Kind "release" or a release pipeline).
func Select(w sched.World, j sched.Job, width int, release bool) Choice {
	c := Choice{Excluded: map[string]string{}}
	if release || j.Pipeline.Release {
		c.Reason = ReasonRelease
		return c
	}
	w = cloneWorld(w)
	for _, r := range w.Hosted {
		c.Excluded[r.ID] = ReasonSpend
	}
	w.Hosted = nil
	free := 0
	for _, r := range w.Runners {
		switch {
		case Spend(w, r):
			c.Excluded[r.ID] = ReasonSpend
		case !Trusted(w, j, r):
			c.Excluded[r.ID] = ReasonTrust
		default:
			free += max(r.Avail.Slots, 0)
			continue
		}
		exclude(&w, r.ID, c.Excluded[r.ID])
	}
	width = min(max(width, 1), MaxWidth, free)
	for len(c.Runners) < width {
		res := sched.Place(w, []sched.Job{j})
		if len(res.Placements) == 0 {
			break
		}
		p := res.Placements[0]
		if p.RunnerID == "" || slices.Contains(c.Runners, p.RunnerID) || c.Excluded[p.RunnerID] != "" {
			break // defensive: the scheduler ignored an exclusion
		}
		c.Runners = append(c.Runners, p.RunnerID)
		w.Apply(p)
		exclude(&w, p.RunnerID, ReasonSibling)
	}
	if !c.Raceable() {
		c.Reason = ReasonNoTarget
	}
	return c
}

// exclude adds a race reason to the runner's exclusions (World.Exclusions).
func exclude(w *sched.World, id, why string) {
	w.Exclusions[id] = append(w.Exclusions[id], model.PlacementReasonDetail{Code: string(model.OverrideExcluded), Detail: why})
}

// cloneWorld copies every part of w that Select or World.Apply writes.
func cloneWorld(w sched.World) sched.World {
	w.Runners = slices.Clone(w.Runners)
	w.Hosted = slices.Clone(w.Hosted)
	ex := make(map[string][]model.PlacementReasonDetail, len(w.Exclusions))
	for k, v := range w.Exclusions {
		ex[k] = slices.Clone(v)
	}
	w.Exclusions = ex
	w.Counts.Runners = cloneMap(w.Counts.Runners)
	w.Counts.Providers = cloneMap(w.Counts.Providers)
	w.Allowances = cloneMap(w.Allowances)
	w.Budgets.Providers = cloneMap(w.Budgets.Providers)
	return w
}

func cloneMap[V any](m map[string]V) map[string]V {
	if m == nil {
		return nil
	}
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
