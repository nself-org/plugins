package isolation

// Purpose: the `isolation-runtime` section of `nself ci doctor` (P7-CI-50 registry): one
// check per registered isolation level, with the profile's reason and install hint
// for a level that is unavailable here (D-1063).
// The cmd package owns the separate `isolation` section (last-verified records).
// Inputs:  the default registry (filled by the run hooks), a probe per profile.
// Outputs: CI-DT001, one per level (a single pass when no profile is registered).
// Ids with prefix CI-DT belong to the trust and isolation Epics.
// Constraints: an unavailable level is a warning, not a failure: only a job that asks
// for the level is blocked (E678). A probe honours ctx; one cut short by it says so
// and reports no verdict.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

// SectionName is the registered section.
const SectionName = "isolation-runtime"

func init() {
	doctor.RegisterSection(doctor.Section{
		Name:     SectionName,
		Selector: doctor.SelectAlways,
		Run:      func(ctx context.Context, _ doctor.Env) []doctor.Check { return doctorChecks(ctx, Default) },
	})
}

// doctorChecks probes every profile of r in level order.
func doctorChecks(ctx context.Context, r *Registry) []doctor.Check {
	base := doctor.Check{ID: "CI-DT001", Name: "isolation level"}
	profiles := r.Profiles()
	if len(profiles) == 0 {
		base.Status, base.Message = doctor.Pass, "no isolation profile registered, jobs run with process isolation"
		return []doctor.Check{base}
	}
	out := make([]doctor.Check, 0, len(profiles))
	for _, p := range profiles {
		c := base
		c.Subject = string(p.Level())
		av := p.Probe(ctx)
		av.Level = Effective(p.Level(), av.Level)
		switch {
		case ctx.Err() != nil:
			c.Status = doctor.Warn
			c.Message = fmt.Sprintf("%s: probe did not finish (%v)", p.Level(), ctx.Err())
			c.Remediation = "Re-run doctor; if it repeats, check that the container runtime responds"
		case av.Available():
			c.Status, c.Message = doctor.Pass, fmt.Sprintf("%s: available (delivers %s)", p.Level(), av.Level)
		default:
			c.Status = doctor.Warn
			c.Message = fmt.Sprintf("%s: unavailable (%s); jobs that need it fail with E678", p.Level(), av.Reason)
			c.Remediation = av.Facts["hint"]
			if c.Remediation == "" {
				c.Remediation = "Install or enable the runtime for the level, or run the job on a runner that has it"
			}
		}
		out = append(out, c)
	}
	return out
}
