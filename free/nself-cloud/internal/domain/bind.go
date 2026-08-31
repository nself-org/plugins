package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/validate"
)

// InstanceRecord is the minimal subset of np_cloud_instances needed by BindDomain.
type InstanceRecord struct {
	ID   string // UUID
	Slug string // <slug>.nself.cloud — used as the expected CNAME target
}

// TenantRecord is the minimal subset of np_cloud_tenants needed by BindDomain.
type TenantRecord struct {
	ID string // UUID
}

// DomainRecord represents a row that will be upserted into np_cloud_domains.
type DomainRecord struct {
	ID            string
	TenantID      string
	InstanceID    string
	Hostname      string
	CNAMETarget   string
	Status        string // "pending" | "verified" | "failed" | "removed"
	DNSVerifiedAt *time.Time
	SSLIssuedAt   *time.Time
	SSLExpiresAt  *time.Time
	SSLStatus     string // "pending" | "issued" | "failed" | "renewing"
}

// DomainRepository is the persistence interface for np_cloud_domains.
type DomainRepository interface {
	// Upsert inserts or updates a domain record. Returns the record with ID filled.
	Upsert(ctx context.Context, r *DomainRecord) (*DomainRecord, error)
}

// Binder orchestrates the full domain binding flow for a tenant instance.
type Binder struct {
	CNAMEVerifier CNAMEVerifier
	LEClient      *LEClient
	DomainRepo    DomainRepository
	// NCloudEdgeSuffix is the base suffix for tenant CNAME targets (e.g. "nself.cloud").
	NCloudEdgeSuffix string
}

// BindDomain executes the four-step domain binding flow:
//  1. Validate hostname (RFC 1123, reserved name checks).
//  2. Verify CNAME points to <instance.Slug>.<NCloudEdgeSuffix>.
//  3. Issue a Let's Encrypt certificate via DNS-01.
//  4. Upsert np_cloud_domains with status=ssl_issued.
//
// On any step failure the domain record is upserted with status="failed" and
// the error is returned so the API layer can surface it to the caller.
func (b *Binder) BindDomain(ctx context.Context, tenant TenantRecord, instance InstanceRecord, hostname string) (*DomainRecord, error) {
	if b.NCloudEdgeSuffix == "" {
		b.NCloudEdgeSuffix = "nself.cloud"
	}

	// Step 1 — Validate hostname.
	if err := validate.ValidateHostname(hostname); err != nil {
		return b.failDomain(ctx, tenant, instance, hostname, fmt.Sprintf("hostname validation: %v", err))
	}

	cnameTarget := fmt.Sprintf("%s.%s", instance.Slug, b.NCloudEdgeSuffix)

	// Step 2 — Verify CNAME.
	if err := b.CNAMEVerifier.VerifyCNAME(ctx, hostname, cnameTarget); err != nil {
		return b.failDomain(ctx, tenant, instance, hostname, fmt.Sprintf("CNAME verification: %v", err))
	}

	// Step 3 — Issue Let's Encrypt cert.
	meta, err := b.LEClient.IssueCert(ctx, tenant.ID, hostname)
	if err != nil {
		return b.failDomain(ctx, tenant, instance, hostname, fmt.Sprintf("TLS certificate issuance: %v", err))
	}

	// Step 4 — Persist success.
	now := time.Now().UTC()
	rec := &DomainRecord{
		TenantID:     tenant.ID,
		InstanceID:   instance.ID,
		Hostname:     hostname,
		CNAMETarget:  cnameTarget,
		Status:       "verified",
		DNSVerifiedAt: &now,
		SSLIssuedAt:  &meta.IssuedAt,
		SSLExpiresAt: &meta.ExpiresAt,
		SSLStatus:    "issued",
	}
	saved, err := b.DomainRepo.Upsert(ctx, rec)
	if err != nil {
		return nil, fmt.Errorf("persist domain record for %q: %w", hostname, err)
	}
	return saved, nil
}

// failDomain upserts a failed domain record and returns the error for the caller.
func (b *Binder) failDomain(ctx context.Context, tenant TenantRecord, instance InstanceRecord, hostname, reason string) (*DomainRecord, error) {
	rec := &DomainRecord{
		TenantID:   tenant.ID,
		InstanceID: instance.ID,
		Hostname:   hostname,
		Status:     "failed",
		SSLStatus:  "failed",
	}
	_, _ = b.DomainRepo.Upsert(ctx, rec) // best-effort; ignore upsert error
	return nil, fmt.Errorf("BindDomain: %s", reason)
}
