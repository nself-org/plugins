package plan

// Purpose: expand the kept jobs into matrix entries, decide each entry's
// state (planned, skip, unplaceable), layer the DAG and fill the Plan.
// Inputs:  the kept jobs with their inclusion reasons.
// Outputs: Plan.Jobs ordered by layer, priority and id; Plan.Layers.
// Constraints: a matrix job's dependants wait for every entry of it; a
// false env_present keeps the job as a skip check (never drops it); an
// unplaceable job stays in the plan with its unplaceable.* reason.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type entryRef struct {
	id, platform string
	empty        bool
}

func entryID(job, platform string) string {
	if platform == "" {
		return job
	}
	return job + "@" + platform
}

func expand(p *Plan, in Input, cs map[string]*cand, ids []string, reasons map[string]model.Reason) error {
	entries := map[string][]entryRef{}
	for _, id := range ids {
		if _, ok := reasons[id]; !ok {
			continue
		}
		list, empty := expandMatrix(cs[id].job, p.Support)
		for _, plat := range list {
			entries[id] = append(entries[id], entryRef{entryID(id, plat), plat, empty})
		}
	}
	nodes := map[string]*node{}
	owner := map[string]string{}
	for id, list := range entries {
		for _, e := range list {
			n := &node{id: e.id, prio: prioRank(string(cs[id].job.Priority))}
			for _, d := range cs[id].job.Deps {
				for _, de := range entries[d] {
					n.deps = append(n.deps, de.id)
				}
			}
			nodes[e.id] = n
			owner[e.id] = id
		}
	}
	at, layers, err := layer(nodes)
	if err != nil {
		return err
	}
	later := dependants(nodes)
	p.Layers = layers
	if p.Layers == nil {
		p.Layers = [][]string{}
	}
	p.Jobs = []PlannedJob{}
	for _, l := range layers {
		for _, eid := range l {
			c := cs[owner[eid]]
			plat := ""
			empty := false
			for _, e := range entries[c.id] {
				if e.id == eid {
					plat, empty = e.platform, e.empty
				}
			}
			pj := PlannedJob{ID: eid, Name: c.name, Platform: plat, Check: c.check, Layer: at[eid],
				State: StatePlanned, Reason: reasons[c.id], Job: cloneJob(c.job),
				Deps: sortedUnique(nodes[eid].deps), Dependants: later[eid]}
			if miss := envMissing(c.job.When, in.Revision); len(miss) > 0 {
				pj.State, pj.Blocked, pj.Detail = StateSkip, "when.env_missing", "environment not set: "+strings.Join(miss, ", ")
			} else if r, detail := unplaceable(c, plat, in.World, empty); r != "" {
				pj.State, pj.Blocked, pj.Detail = StateUnplaceable, r, detail
			}
			p.Jobs = append(p.Jobs, pj)
		}
	}
	return nil
}
