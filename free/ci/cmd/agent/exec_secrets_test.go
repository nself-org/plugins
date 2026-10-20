package main

// Purpose: an isolation profile that wraps JobSpec.Secrets (the Tart vm profile) must see the lease's
// secrets on the agent path (D-1353).
// Inputs:  a recording Isolator installed as agentIsolation and a job that declares one secret.
// Outputs: the JobSpec the executor handed to Prepare.
// Constraints: the isolator refuses the attempt, so nothing runs and no VM or process is needed.

import (
	"context"
	"errors"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type specRecorder struct{ got *ciexec.JobSpec }

func (r *specRecorder) Prepare(_ context.Context, s *ciexec.JobSpec) error {
	c := *s
	r.got = &c
	return errors.New("recorded")
}

func TestAgentPassesLeaseSecretsOnTheJobSpec(t *testing.T) {
	useState(t)
	rec := &specRecorder{}
	old := agentIsolation
	agentIsolation = rec
	t.Cleanup(func() { agentIsolation = old })

	spec := execSpec{AttemptID: "attempt-1", CoordinatorID: "local", JobID: "fixture", Revision: rev40,
		Job: model.Job{Kind: "static", Run: []string{"true"}, Secrets: []string{"API_TOKEN"}}}
	if _, _, err := execute(context.Background(), spec, jobInputs{Secrets: map[string]string{"API_TOKEN": "tok-value"}}); err != nil {
		t.Fatal(err)
	}
	if rec.got == nil || rec.got.Secrets == nil {
		t.Fatalf("the isolator saw no per-attempt secret source: %+v", rec.got)
	}
	vals, err := rec.got.Secrets.Resolve(context.Background(), []string{"API_TOKEN"})
	if err != nil || vals["API_TOKEN"] != "tok-value" {
		t.Fatalf("resolve = %v, %v", vals, err)
	}
}

func TestAgentLeavesJobSpecSecretsNilWithoutSecrets(t *testing.T) {
	useState(t)
	rec := &specRecorder{}
	old := agentIsolation
	agentIsolation = rec
	t.Cleanup(func() { agentIsolation = old })

	spec := execSpec{AttemptID: "attempt-2", CoordinatorID: "local", JobID: "fixture", Revision: rev40,
		Job: model.Job{Kind: "static", Run: []string{"true"}}}
	if _, _, err := execute(context.Background(), spec, jobInputs{}); err != nil {
		t.Fatal(err)
	}
	if rec.got == nil || rec.got.Secrets != nil {
		t.Fatalf("a secret-free job must carry no source: %+v", rec.got)
	}
}
