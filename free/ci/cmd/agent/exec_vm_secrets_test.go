package main

import (
	"context"
	"errors"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

type vmSecretProbe struct {
	seen bool
	want bool
}

func (p *vmSecretProbe) Prepare(ctx context.Context, s *ciexec.JobSpec) error {
	p.seen = true
	if (s.Secrets != nil) != p.want {
		return errors.New("wrong per-attempt source at VM preparation")
	}
	if p.want {
		values, err := s.Secrets.Resolve(ctx, s.Job.Secrets)
		if err != nil || values["fixture"] != "fixture-value" {
			return errors.New("VM preparation cannot resolve declared secret")
		}
	}
	return errors.New("VM preparation inspected")
}

func TestAgentVMReceivesPerAttemptSecrets(t *testing.T) {
	useState(t)
	old := cirun.Hooks{}.Resolved().Isolation
	t.Cleanup(func() { cirun.RegisterIsolation(old) })
	for _, declared := range []bool{false, true} {
		t.Run(map[bool]string{false: "none", true: "declared"}[declared], func(t *testing.T) {
			probe := &vmSecretProbe{want: declared}
			cirun.RegisterIsolation(probe)
			spec := execSpec{AttemptID: "vm-secret", CoordinatorID: "c", JobID: "j", Revision: rev40, Job: model.Job{Isolation: "vm", Run: []string{"true"}}}
			in := jobInputs{}
			if declared {
				spec.Job.Secrets = []string{"fixture"}
				in.Secrets = map[string]string{"fixture": "fixture-value"}
			}
			res, _, err := execute(context.Background(), spec, in)
			if err != nil || !probe.seen || res.Err == nil || res.Err.Error() != "VM preparation inspected" {
				t.Fatalf("agent VM preparation: seen=%v err=%v result=%v", probe.seen, err, res.Err)
			}
		})
	}
}
