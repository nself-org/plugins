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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/drill"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// placementClock is the world's clock; tests pin it.
var placementClock = time.Now

// placementProbe reads the local host's capacity; tests pin it so the plan
// does not depend on the machine's load, memory or battery.
var placementProbe = ciexec.Probe

func init() { placePlan = planPlacement }

// planSpec selects the plan to place.
type planSpec struct{ repo, cfg, base, pipeline, trigger string }

// planned is a built plan with the facts placement needs.
type planned struct {
	interactive bool
	root        string
	pl          *plan.Plan
	world       plan.World
	checks      []plan.Check
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
	var down *storeDown
	if errors.As(err, &down) {
		// The plan itself is still good: say why placement is missing, loudly, and leave data.placement out.
		// Any other failure (an invalid policy, a world that cannot be built) is a real error and fails the plan.
		fmt.Fprintln(os.Stderr, "ci plan: placement unavailable: "+down.Error())
		return nil, nil, nil
	}
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
		if out[i] == "" { // not sent to the scheduler: a skip check
			out[i] = j.State + ": " + string(j.Blocked)
		}
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
	if cfg, err = drill.Expand(root, cfg, s.pipeline, s.trigger); err != nil { // as a real run: drills_dir subdirectories become kind drill jobs (D-1046)
		return planned{}, err
	}
	rev := planRevision(ctx, root, s.base)
	facts, checks, err := planChecks(root, cfg, fullSelection(rev))
	if err != nil {
		return planned{}, err
	}
	w := planWorld(cfg, checks)
	pl, err := plan.Build(plan.Input{Config: cfg, Facts: facts, Checks: checks, Revision: rev,
		World: w, Pipeline: s.pipeline, Trigger: model.Trigger(s.trigger)})
	if err != nil {
		return planned{}, err
	}
	return planned{interactive: interactiveRun(s.trigger, os.Getenv), root: root, pl: pl, world: w, checks: checks}, nil
}

// storeDown marks a run store that could not be opened: the one failure
// `ci plan` survives without placement.
type storeDown struct{ err error }

func (e *storeDown) Error() string { return "the run store cannot be opened: " + e.err.Error() }
func (e *storeDown) Unwrap() error { return e.err }

// placementDoc places the plan's jobs and returns the v1 document.
func placementDoc(ctx context.Context, pd planned) (model.PlacementPlan, error) {
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return model.PlacementPlan{}, &storeDown{err}
	}
	defer func() { _ = st.Close() }()
	res := policy.Resolver{Store: st, Run: policy.Runner(report.Capture)}
	project := policy.ProjectIDGitCached(ctx, res.Run, pd.root)
	values, err := schedRevisionValues()
	if err != nil {
		return model.PlacementPlan{}, err
	}
	got, err := res.Resolve(ctx, policy.Repo{Root: pd.root, Project: project, Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: pd.root, Run: values})
	if err != nil {
		return model.PlacementPlan{}, err
	}
	sp, err := schedpolicy.Placement(&got.Effective)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	jobs, blocked := planview.SchedJobs(planview.Context{PipelineID: pd.pl.Pipeline.ID, Project: project, Support: pd.pl.Support,
		DefaultPlatform: pd.world.Platform, Foreground: pd.interactive}, plannedJobs(pd))
	capacity := placementProbe()
	now := placementClock()
	pipe := sched.Pipeline{ID: pd.pl.Pipeline.ID}
	if len(jobs) > 0 {
		pipe = jobs[0].Pipeline
	}
	deps := world.Deps{Store: st, Jobs: jobs, LocalCapacity: &capacity, Clock: func() time.Time { return now },
		Policy: func(context.Context, string) (sched.Policy, error) { return sp, nil }}
	w, digest, err := world.BuildForLocal(ctx, deps, world.LocalInput{Project: project, Platform: pd.world.Platform, Capacity: capacity,
		Docker: pd.world.Docker, Tools: pd.world.Tools, Now: now}, pipe) // the same builder a run uses (P7-AUTO-28)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	rev := ""
	if out, ok := planGit(ctx, pd.root, "rev-parse", "HEAD"); ok { // no commit or no git: the revision is unknown, not git's message
		rev = strings.TrimSpace(out)
	}
	return planview.Build(planview.Input{Name: pd.pl.Pipeline.Name, Revision: rev, Trigger: string(pd.pl.Trigger),
		PolicyDigest: string(got.Digest), WorldDigest: digest, World: w, Jobs: jobs, Blocked: blocked, Result: sched.Place(w, jobs)}), nil
}

func plannedJobs(pd planned) []planview.PlannedJob {
	pl := pd.pl
	tools := map[string]string{}
	for _, c := range pd.checks {
		tools[c.ID] = c.Tool
	}
	out := make([]planview.PlannedJob, len(pl.Jobs))
	for i, j := range pl.Jobs {
		out[i] = planview.PlannedJob{ID: j.ID, Name: j.Name, Platform: j.Platform, Layer: j.Layer, Deps: j.Deps, Job: j.Job,
			Tool: tools[j.Check], State: j.State, Blocked: string(j.Blocked), Detail: j.Detail}
	}
	return out
}
