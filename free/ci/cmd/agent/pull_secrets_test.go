package main

// Purpose: the node-side resolver judges eligibility by the job's trust class from the lease (D-1373): a class
//          the node cannot hold secrets for, or none, resolves nothing and runs nothing.
// Constraints: no process is started; the refusal comes before the first resolver call.

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

func TestNodeResolverRefusesAJobWithoutATrustedClass(t *testing.T) {
	f := pullFlags{project: t.TempDir(), secretsOK: true, dedicated: true, coordinator: "c1"}
	resolve := nodeResolver(t.TempDir(), f, "n-1", "/nonexistent/age.key")
	for _, class := range []model.TrustClass{"untrusted", "", "not-a-class"} {
		_, err := resolve(context.Background(), "a1", class, []string{"project/API_TOKEN"})
		var se *secrets.Error
		if !errors.As(err, &se) || se.Code != "E677" {
			t.Fatalf("class %q: err = %v, want E677", class, err)
		}
	}
}
