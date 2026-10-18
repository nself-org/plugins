package main

// Purpose: placement for `nself ci plan` and `nself ci explain` (P7-SCHED-03):
// build the plan's jobs, the world snapshot and the effective scheduling
// policy, run the pure scheduler over them in the plan's topological order and
// turn the result into the placement-plan v1 document.
// Inputs: the plan flags (--repo, --config, --base, --pipeline, --trigger),
// env NSELF_CI_REPO, NSELF_CI_HOME; the state.db read-only in effect.
// Outputs: data.placement of `plan --json` and a PLACEMENT table column.
// Constraints: V15 only (planFlags and the hook live behind the V15 table);
// creates no lease, reservation, ledger row or decision record; no secret
// value is read (job secrets map to their class name); the placePlan hook
// receives a model.Pipeline without job ids, so the plan is rebuilt from the
// same flags and checked against it (Epic Invalidations, P7-SCHED-03).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/config"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// placementClock is the world's clock; tests pin it.
var placementClock = time.Now

func init() { placePlan = planPlacement }

// planSpec selects the plan to place.
type planSpec struct{ repo, cfg, base, pipeline, trigger string }

// planned is a built plan with the facts placement needs.
type planned struct {
	root  string
	pl    *plan.Plan
	world plan.World
}

func planPlacement(ctx context.Context, p *model.Pipeline) (any, []string, error) {
	pd, err := loadPlanned(ctx, planSpec{planOpts.repo, planOpts.cfg, planOpts.base, planOpts.pipeline, planOpts.trigger})
	if err != nil {
		return nil, nil, err
	}
	if !samePlan(pd.pl, p) {
		return nil, nil, errors.New("the plan changed while placement was computed; run it again")
	}
	doc, err := placementDoc(ctx, pd)
	if err != nil {
		return nil, nil, err
	}
	return doc, cellsInPlanOrder(pd.pl, doc), nil
}

// samePlan checks the rebuilt plan against the one the hook was handed: same
// jobs, in the same order, with the same commands and dependencies.
func samePlan(pl *plan.Plan, p *model.Pipeline) bool {
	if len(pl.Jobs) != len(p.Jobs) {
		return false
	}
	for i, j := range pl.Jobs {
		if j.Job.Kind != p.Jobs[i].Kind || !slices.Equal(j.Job.Run, p.Jobs[i].Run) || !slices.Equal(j.Job.Deps, p.Jobs[i].Deps) {
			return false
		}
	}
	return true
}

// cellsInPlanOrder returns the PLACEMENT cells in the order of the plan's
// job table, which is not the order jobs were placed in.
func cellsInPlanOrder(pl *plan.Plan, doc model.PlacementPlan) []string {
	byID := map[string]string{}
	for i, c := range planview.Cells(doc) {
		byID[doc.Jobs[i].ID] = c
	}
	out := make([]string, len(pl.Jobs))
	for i, j := range pl.Jobs {
		out[i] = byID[j.ID]
	}
	return out
}

// loadPlanned repeats the plan steps of runPlan (cmd/plan.go is not edited).
func loadPlanned(ctx context.Context, s planSpec) (planned, error) {
	root := firstOf(s.repo, os.Getenv("NSELF_CI_REPO"))
	var err error
	if root == "" {
		root, err = os.Getwd()
	} else {
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return planned{}, err
	}
	if err = config.CheckLeftover(root); err != nil {
		return planned{}, err
	}
	cfg := model.PipelineConfig{Version: 1}
	loaded, err := config.Load(firstOf(s.cfg, filepath.Join(root, config.FileName)))
	switch {
	case err == nil:
		cfg = loaded.Config
	case !errors.Is(err, os.ErrNotExist) || s.cfg != "":
		return planned{}, err
	}
	facts, checks, err := planChecks(root, cfg)
	if err != nil {
		return planned{}, err
	}
	w := planWorld(cfg, checks)
	pl, err := plan.Build(plan.Input{Config: cfg, Facts: facts, Checks: checks, Revision: planRevision(ctx, root, s.base),
		World: w, Pipeline: s.pipeline, Trigger: model.Trigger(s.trigger)})
	if err != nil {
		return planned{}, err
	}
	return planned{root: root, pl: pl, world: w}, nil
}

// placementDoc places the plan's jobs and returns the v1 document.
func placementDoc(ctx context.Context, pd planned) (model.PlacementPlan, error) {
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return model.PlacementPlan{}, err
	}
	defer func() { _ = st.Close() }()
	res := policy.Resolver{Store: st, Run: policy.Runner(report.Capture)}
	project := policy.ProjectIDGit(ctx, res.Run, pd.root)
	got, err := res.Resolve(ctx, policy.Repo{Root: pd.root, Project: project, Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: pd.root})
	if err != nil {
		return model.PlacementPlan{}, err
	}
	sp, err := schedpolicy.Placement(&got.Effective)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	jobs := planview.SchedJobs(planview.Context{PipelineID: pd.pl.Pipeline.ID, Project: project, Support: pd.pl.Support,
		DefaultPlatform: pd.world.Platform, Foreground: foregroundTrigger(string(pd.pl.Trigger))}, plannedJobs(pd.pl))
	snap, err := st.WorldSnapshot(ctx)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	capacity := ciexec.Probe()
	now := placementClock()
	var local *model.Capability
	if !hasNode(snap.Nodes, "local") { // a registered "local" node is the local runner
		c, err := planview.LocalCapability(planview.LocalFacts{Project: project, Platform: pd.world.Platform, CPUs: capacity.CPUs,
			MemMB: int64(capacity.MemoryMB), Docker: pd.world.Docker, Tools: pd.world.Tools, Now: now})
		if err != nil {
			return model.PlacementPlan{}, err
		}
		local = &c
	}
	pipe := sched.Pipeline{ID: pd.pl.Pipeline.ID}
	if len(jobs) > 0 {
		pipe = jobs[0].Pipeline
	}
	w, _, err := world.Build(ctx, world.Deps{Store: st, Local: local, LocalCapacity: &capacity, Jobs: jobs, Clock: func() time.Time { return now },
		Policy: func(context.Context, string) (sched.Policy, error) { return sp, nil }}, pipe)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	if local != nil {
		planview.MarkPersonalLocal(&w)
	}
	digest, err := world.Digest(w)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	rev := ""
	if out, ok := planGit(ctx, pd.root, "rev-parse", "HEAD"); ok { // no commit or no git: the revision is unknown, not git's message
		rev = strings.TrimSpace(out)
	}
	return planview.Build(planview.Input{Name: pd.pl.Pipeline.Name, Revision: rev, Trigger: string(pd.pl.Trigger),
		PolicyDigest: string(got.Digest), WorldDigest: digest, World: w, Jobs: jobs, Result: sched.Place(w, jobs)}), nil
}

func plannedJobs(pl *plan.Plan) []planview.PlannedJob {
	out := make([]planview.PlannedJob, len(pl.Jobs))
	for i, j := range pl.Jobs {
		out[i] = planview.PlannedJob{ID: j.ID, Name: j.Name, Platform: j.Platform, Layer: j.Layer, Deps: j.Deps, Job: j.Job}
	}
	return out
}

func hasNode(nodes []store.SchedNode, id string) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}
