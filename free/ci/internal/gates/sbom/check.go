// Package sbom registers the `sbom` check kind with the check catalogue
// (P7-CI-34) and runs it (P7-CACHE-08, Epic CACHE D13).
package sbom

// Purpose: the `sbom` check: required and substantive for a release job,
// advisory elsewhere, plus the implicit artifact declaration of its outputs.
// Inputs:  a model.Job, the pipeline config, the SBOM targets.
// Outputs: gates.CheckDef (Register), model.Check (Run), model.ArtifactDecl (Implicit).
// Constraints: a release job must run the check as its last step, otherwise
// E718 invalid refuses the run; the implicit `.nself-ci/sbom/` declaration is
// added only for a job that runs the check, so other jobs never fail on a
// declaration that selects nothing; the kind is `sbom`.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

// CheckID is the catalogue id a job lists in run: to inventory its artifacts.
const CheckID = "release.sbom"

// Kind is the check kind recorded in evidence.
const Kind = "sbom"

func init() {
	records.RegisterImplicit(Implicit)
}

// Def is the catalogue row. Whether the check is required is decided per job
// by Required (a release job is always required); the row is advisory.
func Def() gates.CheckDef {
	return gates.CheckDef{
		ID: CheckID, Kind: Kind, Tool: sbom.ToolName,
		Argv:            []string{sbom.ToolName, "scan", "{target}", "-o", sbom.FormatCycloneDX, "-o", sbom.FormatSPDX},
		VersionProbe:    []string{sbom.ToolName, "version"},
		RequiredDefault: false, Substantive: true, Path: "deep",
		Remediation: "Run `" + sbom.InstallCommand + "` to place the pinned syft",
	}
}

// Register adds the sbom check to a catalogue.
func Register(r *gates.Registry) error { return r.Register(Def()) }

// Required reports whether a failing sbom check fails its job: always for a
// release job, otherwise only under the strict preset or an explicit
// `required: true` on the check.
func Required(kind model.JobKind, cfg model.PipelineConfig) bool {
	if kind == "release" {
		return true
	}
	for _, p := range cfg.Presets {
		if p == "strict" {
			return true
		}
	}
	if c, ok := cfg.Checks[CheckID]; ok && c.Required != nil {
		return *c.Required
	}
	return false
}

// RunsCheck reports whether any step of the job names the sbom check.
func RunsCheck(job model.Job) bool {
	for _, step := range job.Run {
		if stepRuns(step) {
			return true
		}
	}
	return false
}

func stepRuns(step string) bool {
	for _, tok := range strings.Fields(step) {
		if tok == CheckID {
			return true
		}
	}
	return false
}

// RequireForRelease refuses a release job that does not end with the sbom check.
func RequireForRelease(job model.Job) error {
	if job.Kind != "release" {
		return nil
	}
	if !RunsCheck(job) {
		return sbom.Failf(sbom.ReasonInvalid, "a release job must run the %s check as its last step", CheckID)
	}
	if !stepRuns(job.Run[len(job.Run)-1]) {
		return sbom.Failf(sbom.ReasonInvalid, "the %s check must be the last step of a release job", CheckID)
	}
	return nil
}

// Implicit declares the SBOM outputs of a job that runs the check.
func Implicit(job model.Job) []model.ArtifactDecl {
	if !RunsCheck(job) {
		return nil
	}
	return []model.ArtifactDecl{{Path: sbom.OutDir + "/", Kind: "sbom"}}
}
