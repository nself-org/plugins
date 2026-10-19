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

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, item := range env {
		k, v, _ := strings.Cut(item, "=")
		m[k] = v
	}
	return m
}

// TestReleaseEnvCarriesTheKeyAndNoDeployVariable: the environment the executor
// builds from the operator's own (allowlist) plus the release rules has the key
// and no DEPLOY_* or NSELF_DEPLOY_* variable the operator had set.
func TestReleaseEnvCarriesTheKeyAndNoDeployVariable(t *testing.T) {
	t.Setenv("DEPLOY_TOKEN", "operator-secret")
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "/operator/key")
	t.Setenv("PATH", "/usr/bin")
	base, err := ciexec.AllowlistEnv{}.Environment(context.Background(), model.Job{Kind: "release"})
	if err != nil {
		t.Fatal(err)
	}
	key := job.IdempotencyKey("run-1.publish-1.a1")
	env := envMap(job.ReleaseEnv(append(base, "NSELF_DEPLOY_X=1", "deploy_y=1", job.KeyEnv+"=stale"), key))
	if env[job.KeyEnv] != key {
		t.Fatalf("%s = %q", job.KeyEnv, env[job.KeyEnv])
	}
	if env["PATH"] != "/usr/bin" {
		t.Fatal("the allowlisted environment was dropped")
	}
	for k := range env {
		if job.IsDeployVar(k) {
			t.Errorf("deploy variable %s reached the release job", k)
		}
	}
	if n := strings.Count(strings.Join(job.ReleaseEnv([]string{job.KeyEnv + "=old"}, key), "\n"), job.KeyEnv); n != 1 {
		t.Fatalf("the key variable appears %d times", n)
	}
}
