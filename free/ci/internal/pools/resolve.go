package pools

// Purpose: the effective pool set (AUTO D10): pools.allow ∩ the pools the person is enrolled in
//          ∩ pools.dev (dev runs only), and for a named project ∩ the nodes whose
//          lifecycle.authorized_projects lists it (placement already enforces that term on its own).
// Inputs:  Input: the runners with their assigned labels, the terms, the project and the run kind.
// Outputs: Result: the pools (contract:ci.pool v1), the eligible runner ids and, per excluded runner,
//          one pool.excluded reason naming the first term that emptied its pools.
// Constraints: pure. An unset term constrains nothing; a set term keeps a runner only through a pool it
//          lists, so removing a term never shrinks the result. A runner's candidate pools only ever
//          shrink by intersection: no union of pool sets is computed.

import (
	"slices"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Runner is one runner the resolver sees.
type Runner struct {
	ID       string
	Location string
	// Source is SourceLocal, SourceRegistry, SourceCoordinator or SourceProvider.
	Source string
	// Assigned holds the coordinator-assigned labels in placement form. Self-declared labels never go here.
	Assigned []string
	// Projects is lifecycle.authorized_projects; read only when Input.Project is set.
	Projects []string
}

// Input is what Resolve needs.
type Input struct {
	Runners []Runner
	// Allow is pools.allow; Enrolled the pools the person is enrolled in (unset: every pool they see).
	Allow, Enrolled List
	// Dev marks a dev run: only then does DevPools (pools.dev) apply.
	Dev      bool
	DevPools List
	// Project, when set, applies the authorized_projects term.
	Project string
}

// Result is the answer of Resolve.
type Result struct {
	Pools []Pool
	// Eligible lists the runners every term keeps, sorted.
	Eligible []string
	// Excluded holds the pool.excluded reason of every runner a pool term drops.
	Excluded map[string][]model.PlacementReasonDetail
	// Unauthorized lists the runners kept by the pool terms whose authorized projects omit Input.Project.
	Unauthorized []string
}

// Memberships returns the pool ids of one runner: its assigned `pool.<id>` labels and its implicit pool.
func Memberships(r Runner) []string {
	ids := FromAssigned(r.Assigned)
	switch r.Source {
	case SourceLocal:
		ids = append(ids, Local)
	case SourceProvider:
		ids = append(ids, Hosted)
	}
	slices.Sort(ids)
	return ids
}

type term struct {
	list   List
	detail string
}

func (in Input) terms() []term {
	ts := []term{{in.Allow, DetailNotInProjectList}, {in.Enrolled, DetailNotEnrolled}}
	if in.Dev {
		ts = append(ts, term{in.DevPools, DetailDevNotAllowed})
	}
	return ts
}

// keep returns the pools that pass every set term and the detail of the first term that emptied them.
func keep(ids []string, ts []term) ([]string, string) {
	for _, t := range ts {
		if !t.list.Set {
			continue
		}
		ids = slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return !t.list.Has(id) })
		if len(ids) == 0 {
			return nil, t.detail
		}
	}
	return ids, ""
}

// Resolve computes the effective set.
func Resolve(in Input) Result {
	res := Result{Pools: []Pool{}, Eligible: []string{}, Excluded: map[string][]model.PlacementReasonDetail{}, Unauthorized: []string{}}
	ts := in.terms()
	byPool := map[string]*Pool{}
	for _, r := range in.Runners {
		ids := Memberships(r)
		for _, id := range ids {
			p := byPool[id]
			if p == nil {
				p = &Pool{Schema: SchemaID, ID: id, Scope: ScopeOperator, Members: []Member{}}
				byPool[id] = p
			}
			p.Members = append(p.Members, Member{RunnerID: r.ID, Location: r.Location, Source: r.Source})
			if r.Source == SourceCoordinator {
				p.Scope = ScopeProject
			}
		}
		if _, why := keep(ids, ts); why != "" {
			res.Excluded[r.ID] = []model.PlacementReasonDetail{{Code: string(model.PoolExcluded), Detail: why}}
			continue
		}
		if in.Project != "" && !slices.Contains(r.Projects, in.Project) {
			res.Unauthorized = append(res.Unauthorized, r.ID)
			continue
		}
		res.Eligible = append(res.Eligible, r.ID)
	}
	for id, p := range byPool {
		if id == Hosted {
			p.Scope = ScopeHosted
		}
		sort.Slice(p.Members, func(i, j int) bool { return p.Members[i].RunnerID < p.Members[j].RunnerID })
		p.Effective = Effective{Allowed: true, Reasons: []model.PlacementReasonDetail{}}
		if _, why := keep([]string{id}, ts); why != "" {
			p.Effective = Effective{Reasons: []model.PlacementReasonDetail{{Code: string(model.PoolExcluded), Detail: why}}}
		}
		res.Pools = append(res.Pools, *p)
	}
	sort.Slice(res.Pools, func(i, j int) bool { return res.Pools[i].ID < res.Pools[j].ID })
	sort.Strings(res.Eligible)
	sort.Strings(res.Unauthorized)
	return res
}
