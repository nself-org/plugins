package services_test

// Purpose: the hook adapter maps service names to components through the
// nearest .nself/ci.yaml (P7-CI-42). Lives here, in the engine's test package,
// because the ticket's write scope holds one file in internal/run.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

func TestServiceSpecsUseDeclaredComponents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nservices:\n  db:\n    component: postgres\n    version: 16-alpine\n"
	if err := os.WriteFile(filepath.Join(dir, ".nself", "ci.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	got := run.ServiceSpecs([]string{"db", "redis", "db"})
	if len(got) != 2 {
		t.Fatalf("duplicates must collapse: %+v", got)
	}
	if got[0] != (services.Spec{Name: "db", Component: "postgres", Version: "16-alpine"}) {
		t.Fatalf("declared service not mapped: %+v", got[0])
	}
	if got[1] != (services.Spec{Name: "redis"}) {
		t.Fatalf("an undeclared name is a component name: %+v", got[1])
	}
}
