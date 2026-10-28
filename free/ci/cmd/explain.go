package main

// Purpose: `nself ci explain <job>|<run>/<job> [--json]` (P7-SCHED-03, Epic
// SCHED D3/D4): why a job runs where it does, with the chosen runner and every
// alternative's reason codes or score components.
// Inputs: one target; --json, --repo, --config, --base, --pipeline, --trigger;
// env NSELF_CI_REPO, NSELF_CI_HOME.
// Outputs: lines, or one v1 envelope whose data is the placement-plan v1
// document with the jobs asked about and `source` plan|decision-record; exit 0,
// 1 (E692 target not found or no decision record, usage), 2 (infrastructure).
// Constraints: V15 only; read only (creates no lease, reservation, ledger row
// or decision record); a target whose part before the first "/" holds an "@"
// is a planned job id (`build@linux/amd64`), any other target with "/" is
// <run>/<job> (the job may be a matrix id) and goes to the registered
// planview.DecisionSource; a bare name matches the planned
// job with that id or every matrix entry of that name.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
)

var explainFlags = flag.NewFlagSet("explain", flag.ContinueOnError)

var explainOpts struct {
	json                               bool
	pipeline, trigger, repo, cfg, base string
}

func init() {
	explainFlags.BoolVar(&explainOpts.json, "json", false, "print one v1 JSON envelope")
	explainFlags.StringVar(&explainOpts.pipeline, "pipeline", "", "named pipeline to plan (default: every non-drill job)")
	explainFlags.StringVar(&explainOpts.trigger, "trigger", "", "plan the pipelines listing this trigger")
	explainFlags.StringVar(&explainOpts.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	explainFlags.StringVar(&explainOpts.cfg, "config", "", "config file (default: <repo>/.nself/ci.yaml)")
	explainFlags.StringVar(&explainOpts.base, "base", "", "base commit for affected selection (env NSELF_CI_BASE_SHA)")
	registerV15("explain", subcommandEntry{V15Only: true, Flags: func() *flag.FlagSet { return explainFlags }, Handler: runExplain})
}

// explainFail prints the error and returns its exit code.
func explainFail(asJSON bool, err error) int {
	code, exit := "E619", 2
	var pe *planview.Error
	var pl *plan.Error
	var ce *config.Error
	switch {
	case errors.As(err, &pe):
		code, exit = pe.Code, 1
	case errors.As(err, &pl):
		code, exit = pl.Code, 1
	case errors.As(err, &ce):
		code, exit = ce.Code, 1
	}
	if asJSON {
		_ = output.WriteError(os.Stdout, "ci explain", output.ErrorDetail{Code: code, Message: err.Error(), ExitCode: exit}, nil)
	} else {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	return exit
}

// parseInterleaved parses flags that may follow or precede the positionals.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		pos, args = append(pos, rest[0]), rest[1:]
	}
}

func runExplain(args []string) int {
	explainFlags.SetOutput(os.Stderr)
	pos, err := parseInterleaved(explainFlags, args)
	if err != nil {
		return 2
	}
	o := explainOpts
	if len(pos) != 1 || pos[0] == "" {
		return explainFail(o.json, &planview.Error{Code: planview.CodeExplainTarget, Message: "usage: nself ci explain <job>|<run>/<job> [--json]"})
	}
	ctx := context.Background()
	doc, source, err := explainTarget(ctx, pos[0], planSpec{o.repo, o.cfg, o.base, o.pipeline, o.trigger})
	if err != nil {
		return explainFail(o.json, err)
	}
	if o.json {
		if err := output.WriteData(os.Stdout, "ci explain", planview.Explanation{PlacementPlan: doc, Source: source}, nil); err != nil {
			return explainFail(false, err)
		}
		return 0
	}
	fmt.Fprintln(os.Stdout, strings.Join(planview.ExplainLines(doc, source), "\n"))
	return 0
}

// explainTarget resolves the target to a document and where it came from.
func explainTarget(ctx context.Context, target string, spec planSpec) (model.PlacementPlan, string, error) {
	if run, job, ok := strings.Cut(target, "/"); ok && !strings.Contains(run, "@") {
		if run == "" || job == "" {
			return model.PlacementPlan{}, "", &planview.Error{Code: planview.CodeExplainTarget, Message: fmt.Sprintf("explain target %q: name a run and a job as <run>/<job>", target)}
		}
		doc, err := planview.Lookup(ctx, run, job)
		return doc, planview.SourceDecisionRecord, err
	}
	pd, err := buildPlanned(ctx, spec)
	if err != nil {
		return model.PlacementPlan{}, "", err
	}
	doc, err := placementDoc(ctx, pd)
	if err != nil {
		return model.PlacementPlan{}, "", err
	}
	one := planview.Select(doc, func(j model.JobPlacement) bool { return j.ID == target || j.Name == target })
	if len(one.Jobs) == 0 {
		for _, j := range pd.pl.Jobs {
			if (j.ID == target || j.Name == target) && j.State == plan.StateSkip {
				return model.PlacementPlan{}, "", &planview.Error{Code: planview.CodeExplainTarget,
					Message: fmt.Sprintf("job %q is skipped by the plan (%s: %s) and needs no runner", target, j.Blocked, j.Detail)}
			}
		}
		return model.PlacementPlan{}, "", &planview.Error{Code: planview.CodeExplainTarget,
			Message: fmt.Sprintf("no job %q in the current plan; jobs: %s", target, jobIDs(doc))}
	}
	return one, planview.SourcePlan, nil
}

func jobIDs(doc model.PlacementPlan) string {
	ids := make([]string, len(doc.Jobs))
	for i, j := range doc.Jobs {
		ids[i] = j.ID
	}
	return strings.Join(ids, ", ")
}
