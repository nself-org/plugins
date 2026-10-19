package run

// D-1013: every resolved job secret and every generated service credential
// reaches the process-wide registry, so the artifact and cache scanners (which
// read Redactions) are not checking an empty registry.

import (
	"context"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

type fixedSecrets map[string]string

func (f fixedSecrets) Resolve(context.Context, []string) (map[string]string, error) { return f, nil }

func TestExecutorRegistersResolvedJobSecrets(t *testing.T) {
	const val = "d1013-job-secret-6b1e0c47"
	ex := newExecutor(nil, nil, nil)
	ex.Secrets = fixedSecrets{"DEPLOY_PW": val}
	if len(Redactions.Scan([]byte(val))) != 0 {
		t.Fatal("precondition: the value must not be registered yet")
	}
	res := ex.Run(context.Background(), ciexec.JobSpec{AttemptID: "d1013a", CoordinatorID: "coord", Home: t.TempDir(),
		Command: []string{"sh", "-c", "true"}})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("result %+v", res)
	}
	if len(Redactions.Scan([]byte("x "+val+" y"))) == 0 {
		t.Fatal("a resolved job secret was not registered with run.Redactions")
	}
}

func TestExecutorRefusesAJobSecretItCannotRegister(t *testing.T) {
	ex := newExecutor(nil, nil, nil)
	ex.Secrets = fixedSecrets{"PIN": "12345"}
	res := ex.Run(context.Background(), ciexec.JobSpec{AttemptID: "d1013b", CoordinatorID: "coord", Home: t.TempDir(),
		Command: []string{"sh", "-c", "true"}})
	if res.Err == nil || res.Attempt.State == "passed" {
		t.Fatalf("a secret that cannot be masked must fail the attempt, got %+v", res)
	}
	if strings.Contains(res.Err.Error(), "12345") {
		t.Fatalf("the error names the value: %v", res.Err)
	}
}

func TestServiceCredentialsReachTheRegistry(t *testing.T) {
	const val = "d1013-service-credential-4c2a9e"
	if services.SecretSink == nil {
		t.Fatal("services.SecretSink is not wired to the redaction registry")
	}
	services.SecretSink(val)
	if len(Redactions.Scan([]byte(val))) == 0 {
		t.Fatal("a generated service credential was not registered")
	}
}
