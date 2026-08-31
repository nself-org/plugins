package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/domain"
)

// ── test doubles ─────────────────────────────────────────────────────────────

type stubDomainRepo struct {
	saved *domain.DomainRecord
}

func (r *stubDomainRepo) Upsert(_ context.Context, rec *domain.DomainRecord) (*domain.DomainRecord, error) {
	r.saved = rec
	if rec.ID == "" {
		rec.ID = "generated-uuid"
	}
	return rec, nil
}

type stubCertStore struct{}

func (s *stubCertStore) Put(_ context.Context, _, _ string, _, _ []byte) error { return nil }
func (s *stubCertStore) Get(_ context.Context, _, _ string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func makeBinder(cnameErr error) (*domain.Binder, *stubDomainRepo) {
	repo := &stubDomainRepo{}
	leClient := &domain.LEClient{
		CertStore:        &stubCertStore{},
		CloudflareAPIKey: "stub-key",
		AccountEmail:     "test@nself.org",
		Staging:          true,
	}
	return &domain.Binder{
		CNAMEVerifier:    &mockCNAMEVerifier{returnErr: cnameErr},
		LEClient:         leClient,
		DomainRepo:       repo,
		NCloudEdgeSuffix: "nself.cloud",
	}, repo
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestBindDomain_InvalidHostname(t *testing.T) {
	t.Parallel()
	binder, _ := makeBinder(nil)
	_, err := binder.BindDomain(
		context.Background(),
		domain.TenantRecord{ID: "550e8400-e29b-41d4-a716-446655440000"},
		domain.InstanceRecord{ID: "660e8400-e29b-41d4-a716-446655440001", Slug: "my-instance"},
		"../../../etc/passwd",
	)
	if err == nil {
		t.Error("expected error for invalid hostname, got nil")
	}
}

func TestBindDomain_CNAMEMismatch(t *testing.T) {
	t.Parallel()
	binder, repo := makeBinder(errors.New("CNAME mismatch"))
	_, err := binder.BindDomain(
		context.Background(),
		domain.TenantRecord{ID: "550e8400-e29b-41d4-a716-446655440000"},
		domain.InstanceRecord{ID: "660e8400-e29b-41d4-a716-446655440001", Slug: "my-instance"},
		"app.mycorp.com",
	)
	if err == nil {
		t.Error("expected error for CNAME mismatch, got nil")
	}
	// Even on failure, a record should have been persisted with status=failed.
	if repo.saved == nil {
		t.Error("expected a domain record to be upserted on failure")
	} else if repo.saved.Status != "failed" {
		t.Errorf("expected status=failed, got %q", repo.saved.Status)
	}
}

func TestBindDomain_Success(t *testing.T) {
	t.Parallel()
	binder, repo := makeBinder(nil) // nil = CNAME check passes
	rec, err := binder.BindDomain(
		context.Background(),
		domain.TenantRecord{ID: "550e8400-e29b-41d4-a716-446655440000"},
		domain.InstanceRecord{ID: "660e8400-e29b-41d4-a716-446655440001", Slug: "my-instance"},
		"app.mycorp.com",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil DomainRecord")
	}
	if rec.Status != "verified" {
		t.Errorf("expected status=verified, got %q", rec.Status)
	}
	if rec.SSLStatus != "issued" {
		t.Errorf("expected ssl_status=issued, got %q", rec.SSLStatus)
	}
	if repo.saved.CNAMETarget != "my-instance.nself.cloud" {
		t.Errorf("unexpected CNAME target: %q", repo.saved.CNAMETarget)
	}
}
