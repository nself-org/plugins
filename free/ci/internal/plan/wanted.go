package plan

// Purpose: name the executables a plan may need, so the caller probes only
// those on PATH.
// Inputs:  the config and the catalogue checks.
// Outputs: a sorted, de-duplicated list: docker, each check's tool and every
// declared requires.tools.
// Constraints: pure.

import "github.com/nself-org/plugins/free/ci/internal/model"

// WantedTools lists the executables worth probing for.
func WantedTools(cfg model.PipelineConfig, checks []Check) []string {
	want := []string{"docker"}
	for _, c := range checks {
		want = append(want, c.Tool)
	}
	for _, j := range cfg.Jobs {
		if j.Requirements != nil {
			want = append(want, j.Requirements.Tools...)
		}
	}
	return sortedUnique(want)
}
