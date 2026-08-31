// Package domain handles custom domain binding, CNAME verification, and TLS certificate
// lifecycle for nself-cloud tenant instances.
package domain

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// CNAMEVerifier verifies that a hostname's CNAME record points to the expected target.
type CNAMEVerifier interface {
	VerifyCNAME(ctx context.Context, hostname, expectedTarget string) error
}

// DefaultCNAMEVerifier uses the system DNS resolver.
type DefaultCNAMEVerifier struct {
	// Resolver allows injection in tests (nil = system resolver).
	Resolver *net.Resolver
}

// VerifyCNAME performs a DNS CNAME lookup for hostname and verifies that it resolves
// to expectedTarget (or a subdomain of it). The lookup follows the chain up to one
// level: if hostname is itself a CNAME, its target is compared to expectedTarget.
//
// expectedTarget should be of the form "<slug>.nself.cloud" (no trailing dot required;
// both trailing-dot and plain forms are accepted during comparison).
func (v *DefaultCNAMEVerifier) VerifyCNAME(ctx context.Context, hostname, expectedTarget string) error {
	if hostname == "" {
		return fmt.Errorf("hostname must not be empty")
	}
	if expectedTarget == "" {
		return fmt.Errorf("expectedTarget must not be empty")
	}

	r := v.Resolver
	if r == nil {
		r = net.DefaultResolver
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cname, err := r.LookupCNAME(lookupCtx, hostname)
	if err != nil {
		return fmt.Errorf("DNS CNAME lookup for %q failed: %w", hostname, err)
	}

	// Normalize: strip trailing dots for comparison.
	got := strings.TrimSuffix(strings.ToLower(cname), ".")
	want := strings.TrimSuffix(strings.ToLower(expectedTarget), ".")

	if got != want {
		return fmt.Errorf(
			"CNAME mismatch for %q: got %q, want %q — add a CNAME record pointing to %s",
			hostname, got, want, expectedTarget,
		)
	}
	return nil
}

// NewCNAMEVerifier returns a DefaultCNAMEVerifier with the system resolver.
func NewCNAMEVerifier() *DefaultCNAMEVerifier {
	return &DefaultCNAMEVerifier{}
}
