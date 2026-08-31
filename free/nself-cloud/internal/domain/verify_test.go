package domain_test

import (
	"context"
	"testing"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/domain"
)

// mockCNAMEVerifier is a test double for CNAMEVerifier.
type mockCNAMEVerifier struct {
	returnErr error
}

func (m *mockCNAMEVerifier) VerifyCNAME(_ context.Context, _, _ string) error {
	return m.returnErr
}

func TestVerifyCNAME_EmptyHostname(t *testing.T) {
	t.Parallel()
	v := domain.NewCNAMEVerifier()
	err := v.VerifyCNAME(context.Background(), "", "target.nself.cloud")
	if err == nil {
		t.Error("expected error for empty hostname")
	}
}

func TestVerifyCNAME_EmptyTarget(t *testing.T) {
	t.Parallel()
	v := domain.NewCNAMEVerifier()
	err := v.VerifyCNAME(context.Background(), "app.mycorp.com", "")
	if err == nil {
		t.Error("expected error for empty target")
	}
}
