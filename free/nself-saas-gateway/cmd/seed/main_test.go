package main

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// TestDevAPIKeyFormat guards the deterministic key against drifting out of
// the nsk_<64 chars> shape saas.AuthenticateKey requires.
func TestDevAPIKeyFormat(t *testing.T) {
	if !strings.HasPrefix(DevAPIKey, saas.KeyPrefix) {
		t.Fatalf("dev key missing %q prefix", saas.KeyPrefix)
	}
	if got := len(DevAPIKey) - len(saas.KeyPrefix); got != 64 {
		t.Fatalf("dev key suffix length = %d, want 64", got)
	}
	if !strings.HasPrefix(DevAPIKey, "nsk_dev_local_") {
		t.Fatal("dev key must be recognizable as a local dev key")
	}
}

func TestDevTenantIDIsValidUUID(t *testing.T) {
	if err := uuid.Validate(DevTenantID); err != nil {
		t.Fatalf("dev tenant id invalid: %v", err)
	}
}

func TestIsLocalDatabase(t *testing.T) {
	cases := map[string]bool{
		"postgres://u:p@localhost:5432/db":         true,
		"postgres://u:p@127.0.0.1/db":              true,
		"postgres://u:p@postgres:5432/db":          true,
		"postgres://u:p@5.75.235.42:5432/db":       false,
		"postgres://u:p@db.prod.nself.org:5432/db": false,
	}
	for dbURL, want := range cases {
		if got := isLocalDatabase(dbURL); got != want {
			t.Errorf("isLocalDatabase(%q) = %v, want %v", dbURL, got, want)
		}
	}
}
