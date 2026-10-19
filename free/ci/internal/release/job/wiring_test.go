package job_test

// Purpose: the glue in internal/run/release_hook.go, tested from outside (the
// write scope has no test file in internal/run): the dispatch wrapper, the
// isolator chain and the attempt-start hook are registered and behave.

import (
	"context"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func releaseSpec() cirun.AttemptSpec {
	return cirun.AttemptSpec{Pipeline: &model.Pipeline{Name: "release", Kind: "default"}, Job: &model.Job{Kind: "release", Deps: []string{"build"}}, N: 1}
}

func TestDispatchRefusesReleaseJobsOutsideV15(t *testing.T) {
	t.Setenv("NSELF_V15", "0")
	_, err := cirun.Dispatch(context.Background(), releaseSpec())
	if cirun.CodeOf(err) != job.CodeNotAdmitted || !strings.Contains(err.Error(), job.NoEvidence) {
		t.Fatalf("a release job outside V15: %v", err)
	}
}

func TestDispatchLeavesOtherJobsToThePreviousHook(t *testing.T) {
	t.Setenv("NSELF_V15", "0")
	s := releaseSpec()
	s.Job = &model.Job{Kind: "test"}
	_, err := cirun.Dispatch(context.Background(), s) // no session: the previous hook fails, with its own code
	if err == nil || cirun.CodeOf(err) == job.CodeNotAdmitted {
		t.Fatalf("a non-release job must reach the previous hook: %v", err)
	}
	_, err = cirun.Dispatch(context.Background(), cirun.AttemptSpec{}) // nil job
	if cirun.CodeOf(err) == job.CodeNotAdmitted {
		t.Fatalf("a nil job: %v", err)
	}
}

func TestDispatchNeedsARunSessionForReleaseJobs(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	if _, err := cirun.Dispatch(context.Background(), releaseSpec()); err == nil {
		t.Fatal("a release job without a run session was submitted")
	}
}

func TestIsolatorRefusesDeployCredentialsInTheFinalEnvironment(t *testing.T) {
	iso := cirun.Registered.Isolation
	if iso == nil {
		t.Fatal("no isolator registered")
	}
	for name, j := range map[string]model.Job{
		"env":    {Kind: "release", Env: map[string]string{"DEPLOY_TOKEN": "x"}},
		"secret": {Kind: "release", Secrets: []string{"NSELF_DEPLOY_KEY_PATH"}},
	} {
		err := iso.Prepare(context.Background(), &ciexec.JobSpec{Job: j})
		if cirun.CodeOf(err) != job.CodeNotAdmitted || !strings.Contains(err.Error(), job.DeployEnv) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a job of another kind is not this hook's business; it reaches the next isolator
	err := iso.Prepare(context.Background(), &ciexec.JobSpec{Job: model.Job{Kind: "test", Env: map[string]string{"DEPLOY_TOKEN": "x"}}})
	if cirun.CodeOf(err) == job.CodeNotAdmitted {
		t.Fatalf("a non-release job was judged by the release rules: %v", err)
	}
	if err = iso.Prepare(context.Background(), &ciexec.JobSpec{Job: model.Job{Kind: "release", Env: map[string]string{"GOFLAGS": "-v"}}}); cirun.CodeOf(err) == job.CodeNotAdmitted {
		t.Fatalf("a clean release job: %v", err)
	}
}

func TestAttemptStartRefusesReleaseWithoutTheKey(t *testing.T) {
	run := func(a cirun.AttemptInfo) (refused bool) {
		for _, h := range cirun.Registered.AttemptStarts {
			if err := h.AttemptStart(context.Background(), a); err != nil && strings.Contains(err.Error(), job.NoKey) {
				refused = true
			}
		}
		return refused
	}
	if !run(cirun.AttemptInfo{Job: model.Job{Kind: "release"}}) {
		t.Fatal("a release attempt without the idempotency key was started")
	}
	if run(cirun.AttemptInfo{Job: model.Job{Kind: "release"}, IdempotencyKey: job.IdempotencyKey("a")}) {
		t.Fatal("a release attempt with its key was refused")
	}
	if run(cirun.AttemptInfo{Job: model.Job{Kind: "test"}}) {
		t.Fatal("a non-release attempt was judged by the release rules")
	}
}
