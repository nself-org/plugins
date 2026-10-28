package main

// Purpose: share config, detection, revision and world loading for plan and explain.
// Placement receives this exact built plan and its catalogue checks.

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/drill"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

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

// buildPlanned loads and builds the plan once for plan and explain.
func buildPlanned(ctx context.Context, s planSpec) (planned, error) {
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
