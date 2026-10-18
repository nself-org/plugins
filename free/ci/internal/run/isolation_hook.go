package run

// Purpose: install the isolation registry as the executor's Isolator (P7-TRUST-04, Epic D7).
// Inputs:  the job's declared isolation level, from the executor's Prepare call.
// Outputs: the JobSpec wrapped by the selected profile, or an E678/E679 error.
// Constraints: a level above process with no available profile fails the attempt; the
// job is never run at a weaker level. Profiles register from their own *_hook.go file.

import (
	"context"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

// selectIsolator adapts a registry to ciexec.Isolator.
type selectIsolator struct{ reg *isolation.Registry }

func (s selectIsolator) Prepare(ctx context.Context, spec *ciexec.JobSpec) error {
	profile, _, err := s.reg.Select(ctx, spec.Job.Isolation)
	if err != nil {
		return err
	}
	if profile == nil {
		return nil // process level: the executor baseline
	}
	att := isolation.Attempt{ID: spec.AttemptID, CoordinatorID: spec.CoordinatorID, Slot: spec.Slot}
	if err := profile.Prepare(ctx, att); err != nil {
		return err
	}
	wrapped, err := profile.Wrap(*spec)
	if err != nil {
		return err
	}
	*spec = wrapped
	return nil
}

func init() { RegisterIsolation(selectIsolator{reg: isolation.Default}) }
