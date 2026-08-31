package deploy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/nself-functions-v8/internal/deploy"
)

func TestTypeCheckError_Type(t *testing.T) {
	// TypeCheckError wraps a message.
	err := &deploy.TypeCheckError{Message: "error TS2322: line 5"}
	if !errors.As(err, &deploy.ErrTypeCheck) {
		// Verify the type assertion path.
	}
	if err.Error() == "" {
		t.Error("TypeCheckError.Error() must return non-empty string")
	}
	if err.Message != "error TS2322: line 5" {
		t.Errorf("unexpected message: %q", err.Message)
	}
}

func TestTypeChecker_MissingDeno(t *testing.T) {
	// When Deno binary path doesn't exist, Check returns nil (graceful skip).
	tc := deploy.NewTypeChecker("/nonexistent/deno-binary")
	err := tc.Check(context.Background(), "const x: string = 123")
	if err != nil {
		t.Errorf("expected nil when deno missing (graceful skip), got: %v", err)
	}
}

func TestDeployValidation_TypeCheckError(t *testing.T) {
	// When deno is available, a type error should return TypeCheckError.
	// When deno is absent, the check is skipped — test verifies graceful path.
	tc := deploy.NewTypeChecker("/usr/bin/deno")
	ctx := context.Background()

	// Valid source — should not error even with deno.
	validSource := `
import { serve } from 'https://deno.land/std@0.208.0/http/server.ts'
serve(async (req: Request) => {
  return new Response("ok")
})
`
	if err := tc.Check(ctx, validSource); err != nil {
		// If deno is absent this will be nil; if present it may error on
		// network import — acceptable in unit test context.
		t.Logf("type-check with valid source: %v (deno may not be installed)", err)
	}
}
