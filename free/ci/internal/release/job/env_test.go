package job_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
)

func TestIdempotencyKeyIsStableAndPerAttempt(t *testing.T) {
	sum := sha256.Sum256([]byte("nself-ci-idempotency\x00run-1.publish-1.a1"))
	if got := job.IdempotencyKey("run-1.publish-1.a1"); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("key = %s", got)
	}
	if job.IdempotencyKey("a") == job.IdempotencyKey("b") || len(job.IdempotencyKey("a")) != 64 {
		t.Fatal("keys collide or are the wrong length")
	}
}

func TestIsDeployVar(t *testing.T) {
	for name, want := range map[string]bool{"DEPLOY_TOKEN": true, "deploy_key": true, "NSELF_DEPLOY_KEY_PATH": true, "nself_deploy_ssh_key": true,
		"DEPLOYMENT": false, "NSELF_CI_RUN_ID": false, "MY_DEPLOY_X": false, "PATH": false, "": false} {
		if got := job.IsDeployVar(name); got != want {
			t.Errorf("IsDeployVar(%q) = %v", name, got)
		}
	}
}

func TestCheckDeclaredRefusesDeployCredentials(t *testing.T) {
	if e := job.CheckDeclared(model.Job{Kind: "release", Env: map[string]string{"GOFLAGS": "-v"}, Secrets: []string{"NPM_TOKEN"}}); e != nil {
		t.Fatalf("a clean release job was refused: %v", e)
	}
	e := job.CheckDeclared(model.Job{Kind: "release", Env: map[string]string{"DEPLOY_TOKEN": "x", "OK": "1"}, Secrets: []string{"NSELF_DEPLOY_KEY_PATH"}})
	if reasonOf(t, e) != job.DeployEnv || !strings.Contains(e.Message, "DEPLOY_TOKEN, NSELF_DEPLOY_KEY_PATH") {
		t.Fatalf("refusal = %v", e)
	}
	if e = job.CheckDeclared(model.Job{Env: map[string]string{"deploy_x": "1"}}); reasonOf(t, e) != job.DeployEnv {
		t.Fatalf("a lowercase deploy variable was accepted: %v", e)
	}
}

// TestOperatorDeployVariablesNeverReachAReleaseJob: the environment the executor
// builds from the operator's own (allowlist) holds no DEPLOY_* or
// NSELF_DEPLOY_* variable, and a job that declares one is refused.
func TestOperatorDeployVariablesNeverReachAReleaseJob(t *testing.T) {
	t.Setenv("DEPLOY_TOKEN", "operator-secret")
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "/operator/key")
	t.Setenv("PATH", "/usr/bin")
	base, err := ciexec.AllowlistEnv{}.Environment(context.Background(), model.Job{Kind: "release"})
	if err != nil {
		t.Fatal(err)
	}
	sawPath := false
	for _, item := range base {
		name, _, _ := strings.Cut(item, "=")
		if job.IsDeployVar(name) {
			t.Errorf("deploy variable %s passed the allowlist", name)
		}
		sawPath = sawPath || name == "PATH"
	}
	if !sawPath {
		t.Fatal("the allowlisted environment was dropped")
	}
	if e := job.CheckDeclared(model.Job{Kind: "release", Env: map[string]string{"NSELF_DEPLOY_KEY_PATH": "/x"}}); e == nil {
		t.Fatal("a declared deploy variable was accepted")
	}
}
