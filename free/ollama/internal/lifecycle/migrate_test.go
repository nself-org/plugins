package lifecycle_test

// migrate_test.go: BootMigrations is a no-op without DATABASE_URL and fails fast on an unusable URL.
//
// Purpose: Prove the boot-migration hook never blocks a database-less install and never hides a bad URL.
// Inputs: DATABASE_URL set to "" or to a malformed URL via t.Setenv.
// Outputs: nil for the unset case, a non-nil error for the malformed case.
// Constraints: no database and no network.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ollama/internal/lifecycle"
)

func TestBootMigrations_SkipsWithoutDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if err := lifecycle.BootMigrations(context.Background()); err != nil {
		t.Fatalf("want nil without DATABASE_URL, got %v", err)
	}
}

func TestBootMigrations_RejectsMalformedURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "not a url \x7f")
	if err := lifecycle.BootMigrations(context.Background()); err == nil {
		t.Fatal("want an error for a malformed DATABASE_URL")
	}
}
