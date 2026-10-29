package plan

// Purpose: Build, the pure planner: (config, facts, check catalogue,
// revision context, local world) -> validated Pipeline DAG.
// Stages: validate graph (E616/E617) -> pipeline membership (E619) ->
// `when` conditions -> affected selection (selector hook) -> required-deep
// retention on default-branch and release plans -> expansion and layering.
// Outputs: *Plan; every job in the plan has the reason it is there and every
// job not in the plan appears in Skipped with a reason.
// Constraints: no I/O; deterministic; a required deep job is never dropped
// from a default-branch or release plan by branches, paths, triggers or any
// selector (Epic D28); a false env_present on a job keeps it as a skip check.

import (
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Build plans one pipeline.
func Build(in Input) (*Plan, error) {
	cs, ids := candidates(in)
	if err := validateGraph(cs, ids); err != nil {
		return nil, err
	}
	scope, err := failFastScope(in.Config)
	if err != nil {
		return nil, err
	}
	trig := in.Trigger
	if trig == "" {
		trig = "local"
	}
	if in.Trigger != "" && !in.Trigger.Valid() {
		return nil, fail(CodeUnknownName, "unknown trigger "+string(in.Trigger), []string{string(in.Trigger)})
	}
	pname, kind, members, err := selectPipeline(in, cs, ids, trig)
	if err != nil {
		return nil, err
	}
	rev := in.Revision
	forced := map[string]bool{}
	if rev.DefaultBranch || rev.Release {
		// Epic D28: a default-branch or release plan keeps every required
		// deep job, even one outside the named or trigger-selected pipeline.
		// Only a drill pipeline is exempt (drills never join other plans).
		var seeds []string
		for _, id := range ids {
			c := cs[id]
			if c.required && c.deep && (members[id] || (kind != "drill" && c.job.Kind != "drill")) {
				seeds = append(seeds, id)
			}
		}
		forced = closureOf(cs, seeds)
		for id := range forced {
			members[id] = true
		}
	}
	skipped := xcodeNotes(in)
	skip := func(id string, r model.Reason, detail string) { skipped = append(skipped, Skipped{id, r, detail}) }
	for _, id := range ids {
		if !members[id] {
			skip(id, "pipeline.excluded", "not part of pipeline "+pname)
		}
	}

	live := map[string]bool{}
	excluded := map[string]Skipped{}
	for _, id := range ids {
		if !members[id] {
			continue
		}
		c := cs[id]
		if ok, r, detail := whenPasses(c.job.When, rev, trig); !ok && !forced[id] {
			excluded[id] = Skipped{id, r, detail}
		} else if !forced[id] && advisoryToolMissing(c, in.World) {
			excluded[id] = Skipped{id, "advisory.tool_missing", "tool " + c.tool + " is not on PATH"}
		} else {
			live[id] = true
		}
	}
	// A live required job needs its dependency closure: a dependency's own
	// `when` or advisory skip never drops the required gate (Epic D28). Such
	// a dependency is restorable, but only when the required job is kept; it
	// is not offered to the selector, so it can never be picked on its own.
	restorable := map[string]bool{}
	var needed []string
	for _, id := range ids {
		if live[id] && cs[id].required {
			needed = append(needed, cs[id].job.Deps...)
		}
	}
	for id := range closureOf(cs, needed) {
		if members[id] && !live[id] {
			restorable[id] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, id := range ids {
			for _, d := range cs[id].job.Deps {
				if live[id] && !live[d] && !restorable[d] {
					delete(live, id)
					skip(id, "dep.failed", "dependency "+d+" is not in the plan")
					changed = true
				}
			}
		}
	}

	sel, picked := selectJobs(in, cs, ids, live)
	dead := map[string]bool{}
	var keep, need map[string]bool
	for changed := true; changed; {
		changed = false
		var seeds, req []string
		for _, id := range ids {
			if live[id] && !dead[id] && (picked[id] || forced[id]) {
				seeds = append(seeds, id)
			}
		}
		keep = closureOf(cs, seeds)
		for _, id := range ids {
			if keep[id] && cs[id].required {
				req = append(req, cs[id].job.Deps...)
			}
		}
		need = closureOf(cs, req)
		for _, id := range ids {
			if !live[id] || dead[id] || !keep[id] {
				continue
			}
			for _, d := range cs[id].job.Deps {
				if dead[d] || (restorable[d] && !need[d]) {
					dead[id] = true
					skip(id, "dep.failed", "dependency "+d+" is not in the plan")
					changed = true
					break
				}
			}
		}
	}
	for id := range dead {
		delete(live, id)
	}
	for id := range need {
		if restorable[id] {
			delete(excluded, id)
			live[id] = true
		}
	}
	for _, id := range ids {
		if s, ok := excluded[id]; ok {
			skip(s.Job, s.Reason, s.Detail)
		}
	}
	if sel.Digest == "" {
		// The digest covers the final job set, forced jobs included.
		sel.Digest = selectionDigest(in.Revision.Changed, keep)
	}
	reasons := map[string]model.Reason{}
	for _, id := range ids {
		switch {
		case !live[id]:
		case !keep[id]:
			skip(id, "selection.affected", "not affected by the changed paths")
		case sel.Mode == "full":
			reasons[id] = sel.Reason
		case picked[id]:
			reasons[id] = "selection.affected"
		case forced[id]:
			reasons[id] = "selection.full_required"
		default:
			reasons[id] = "selection.affected"
		}
	}

	p := &Plan{Cache: resolvedCache(in.Config.Cache),
		Pipeline:  model.PipelineRef{ID: pname, Name: pname, Kind: kind},
		Trigger:   trig,
		Selection: sel,
		Support:   supportPolicy(in.Config, in.World),
		FailFast:  scope,
		Skipped:   skipped,
		World:     in.World,
	}
	if err := expand(p, in, cs, ids, reasons); err != nil {
		return nil, err
	}
	sort.SliceStable(p.Skipped, func(i, j int) bool {
		if p.Skipped[i].Job != p.Skipped[j].Job {
			return p.Skipped[i].Job < p.Skipped[j].Job
		}
		return p.Skipped[i].Reason < p.Skipped[j].Reason
	})
	return p, nil
}

// releasePipeline is the pipeline name that selectors must always run in full.
const releasePipeline = "release"

// selectJobs runs the selector over the live jobs. Without a base the answer
// is always full / selection.no_base, whatever selector is installed.
func selectJobs(in Input, cs map[string]*cand, ids []string, live map[string]bool) (model.Selection, map[string]bool) {
	var refs []JobRef
	for _, id := range ids {
		if live[id] {
			refs = append(refs, JobRef{ID: id, Workdir: cs[id].workdir})
		}
	}
	rev := in.Revision
	if !rev.BaseKnown {
		picked := map[string]bool{}
		for _, r := range refs {
			picked[r.ID] = true
		}
		return model.Selection{Mode: "full", Reason: "selection.no_base"}, picked
	}
	return runSelector(SelectionInput{Changed: append([]string(nil), rev.Changed...), BaseKnown: true,
		DefaultBranch: rev.DefaultBranch, Release: rev.Release, ReleasePipeline: in.Pipeline == releasePipeline, Jobs: refs, Facts: append([]detect.Fact(nil), in.Facts...)})
}

// selectPipeline resolves --pipeline / --trigger to the member jobs (with
// their dependencies) and the pipeline name and kind.
func selectPipeline(in Input, cs map[string]*cand, ids []string, trig model.Trigger) (string, model.PipelineKind, map[string]bool, error) {
	cfg := in.Config
	var names []string
	switch {
	case in.Pipeline != "":
		if _, ok := cfg.Pipelines[in.Pipeline]; !ok {
			return "", "", nil, fail(CodeUnknownName, "unknown pipeline "+in.Pipeline, []string{in.Pipeline})
		}
		names = []string{in.Pipeline}
	case in.Trigger != "":
		for _, n := range sortedPipelineNames(cfg) {
			if containsTrigger(cfg.Pipelines[n].Triggers, trig) {
				names = append(names, n)
			}
		}
	}
	var seeds []string
	if len(names) == 0 {
		for _, id := range ids {
			if cs[id].job.Kind != "drill" {
				seeds = append(seeds, id)
			}
		}
		return "default", "default", closureOf(cs, seeds), nil
	}
	for _, n := range names {
		for _, id := range cfg.Pipelines[n].Jobs {
			if cs[id] == nil {
				return "", "", nil, fail(CodeUnknownName, "pipeline "+n+" lists unknown job "+id, []string{n, id})
			}
			seeds = append(seeds, id)
		}
	}
	members := closureOf(cs, seeds)
	kind := model.PipelineKind("named")
	if len(members) > 0 {
		kind = "drill"
		for id := range members {
			if cs[id].job.Kind != "drill" {
				kind = "named"
			}
		}
	}
	return joinNames(names), kind, members, nil
}

func sortedPipelineNames(cfg model.PipelineConfig) []string {
	names := make([]string, 0, len(cfg.Pipelines))
	for n := range cfg.Pipelines {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func joinNames(names []string) string {
	out := names[0]
	for _, n := range names[1:] {
		out += "+" + n
	}
	return out
}

// failFastScope reads policy.fail_fast: "dependants" (default) or "pipeline".
func failFastScope(cfg model.PipelineConfig) (string, error) {
	v, ok := cfg.Policy["fail_fast"]
	if !ok {
		return FailFastDependants, nil
	}
	if s, isStr := v.(string); isStr && (s == FailFastDependants || s == FailFastPipeline) {
		return s, nil
	}
	return "", fail(CodeUnknownName, "unknown policy.fail_fast scope (use dependants or pipeline)", nil)
}
