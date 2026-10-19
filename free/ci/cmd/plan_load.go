package main

// Purpose: load the plan `nself ci plan` and `nself ci explain` work on: the
// one place that reads the config, detects the repo, probes the world and calls
// the pure planner (D-0990: placement used to repeat these steps).
// Inputs:  a planSpec (--repo, --config, --base, --pipeline, --trigger; env
// NSELF_CI_REPO, NSELF_CI_BASE_SHA).
// Outputs: the built plan with the world and checks placement needs.
// Constraints: V15 only; every process (git) runs through the exec funnel.

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// planSpec selects the plan to place.
type planSpec struct{ repo, cfg, base, pipeline, trigger string }

// planned is a built plan with the facts placement needs.
type planned struct {
	root   string
	pl     *plan.Plan
	world  plan.World
	checks []plan.Check
}

// loadPlanned builds the plan for a spec: config, detection, revision, world.
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
	return planned{root: root, pl: pl, world: w, checks: checks}, nil
}
