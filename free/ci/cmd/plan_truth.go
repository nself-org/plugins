package main

// Purpose: link the truth check (internal/gates/truth) into the V15 engine
// (D-1240): `checks.truth.rules` in .nself/ci.yaml plans a required `truth`
// job, and the hidden V15 subcommand `nself-ci truth …` is the command that
// job runs, so `nself-ci --job truth` needs no Go toolchain on the runner.
// Inputs:  the pipeline config; for the subcommand, `check|sweep|hash` argv.
// Outputs: a plan.Check (planChecks adds it); the subcommand's exit code
// (0 pass, 1 findings, 2 usage or rule error), one line per rule kind.
// Constraints: no job is planned when another job already runs the check (the
// plugins repo runs it from source), when detect.disable names `truth`, or when
// the config lists no rule files; the argv is the bare name `nself-ci`, so the
// plan stays the same on every host and the tool is a placement requirement.

import (
	"flag"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/gates/truth"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func init() {
	registerV15("truth", subcommandEntry{
		V15Only: true, Hidden: true,
		Flags:   func() *flag.FlagSet { return flag.NewFlagSet("truth", flag.ContinueOnError) },
		Handler: func(args []string) int { return truth.Main(args, os.Stdin, os.Stdout, os.Stderr) },
	})
}

// truthPlanCheck is the catalogue check for checks.truth.rules, or false when
// the config does not call for one.
func truthPlanCheck(cfg model.PipelineConfig) (plan.Check, bool) {
	rules := cfg.Checks[truth.CheckID].Rules
	if len(rules) == 0 || truthDisabled(cfg) || truthRunByAJob(cfg) {
		return plan.Check{}, false
	}
	d := truth.Def()
	return plan.Check{ID: d.ID, Kind: d.Kind, Tool: d.Tool, Workdir: ".",
		Argv:     append([]string{d.Tool, "truth", "check", "--root", "."}, rules...),
		Required: d.RequiredDefault, Substantive: d.Substantive, Path: d.Path, Env: map[string]string{"NSELF_V15": "1"}}, true
}

func truthDisabled(cfg model.PipelineConfig) bool {
	if cfg.Detect == nil {
		return false
	}
	for _, d := range cfg.Detect.Disable {
		if d == truth.CheckID {
			return true
		}
	}
	return false
}

// truthRunByAJob is true when a configured job has a step that names the check.
func truthRunByAJob(cfg model.PipelineConfig) bool {
	for _, j := range cfg.Jobs {
		for _, step := range j.Run {
			if step == truth.CheckID {
				return true
			}
		}
	}
	return false
}
