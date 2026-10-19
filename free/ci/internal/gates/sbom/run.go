package sbom

// Purpose: run the sbom check for one job and map the outcome to a model.Check.
// Inputs:  Input (job, config, runner, home, pin, targets).
// Outputs: Outcome (the evidence check and the generated documents).
// Constraints: a release job's failure is never softened (Result error,
// failure class infra, excerpt "E718 <reason>: ..."); in a non-required job a
// missing or mismatched tool is a skip (advisory.tool_missing) and an invalid
// document is a non-required failure; both carry the E718 reason in the excerpt.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

// Input configures Run.
type Input struct {
	Job     model.Job
	Config  model.PipelineConfig
	Runner  sbom.Runner
	Home    string // NSELF_CI_HOME
	Pin     sbom.Pin
	Targets []sbom.Target
}

// Outcome is what the check produced.
type Outcome struct {
	Check model.Check
	Docs  []sbom.Doc
}

// Run resolves the pinned syft, inventories every target and validates the output.
func Run(ctx context.Context, in Input) Outcome {
	req := Required(in.Job.Kind, in.Config)
	def := Def()
	c := model.Check{ID: CheckID, JobID: CheckID, Kind: Kind, Tool: def.Tool, Required: req, Substantive: def.Substantive}
	if err := RequireForRelease(in.Job); err != nil {
		return Outcome{Check: failed(c, err)}
	}
	mode := sbom.ModeProcess
	if in.Job.Isolation == "container" {
		mode = sbom.ModeContainer
	}
	syft, err := sbom.Resolve(ctx, in.Runner, in.Home, mode, in.Pin)
	if err != nil {
		return Outcome{Check: failed(c, err)}
	}
	v := in.Pin.Version
	c.ToolVersion = &v
	docs, err := sbom.Generate(ctx, in.Runner, syft, in.Pin, in.Targets)
	if err != nil {
		return Outcome{Check: failed(c, err)}
	}
	c.Result = "pass"
	return Outcome{Check: c, Docs: docs}
}

func failed(c model.Check, err error) model.Check {
	c.Excerpt = err.Error()
	if len(c.Excerpt) > 1024 {
		c.Excerpt = c.Excerpt[:1024]
	}
	toolProblem := sbom.IsReason(err, sbom.ReasonToolMissing) || sbom.IsReason(err, sbom.ReasonToolVersion) || sbom.IsReason(err, sbom.ReasonToolChecksum)
	switch {
	case c.Required:
		c.Result, c.FailureClass = "error", "infra"
	case toolProblem:
		c.Result, c.Reason = "skip", "advisory.tool_missing"
	default:
		c.Result, c.FailureClass = "fail", "infra"
	}
	return c
}
