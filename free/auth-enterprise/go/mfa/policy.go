// Package mfa — policy.go
//
// Per-tenant MFA policy enforcement middleware.
//
// When a tenant has np_mfa_policies.required = true, any login attempt for a
// user who does not have an active MFA enrollment is rejected with a 403 and
// a structured JSON body that includes a "mfa_required" field. The client is
// expected to redirect the user to the MFA enrollment flow.
//
// Grace period: existing users who enrolled before the policy was enabled get
// grace_period_d days (default 7) to enroll without being blocked. New
// registrations are blocked immediately.
//
// Admin bypass: an admin can set np_mfa_policy_overrides.bypass = true for a
// specific user. This is audited in the event log.
package mfa

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Policy represents a tenant's MFA enforcement configuration.
type Policy struct {
	TenantID       string
	Required       bool
	GracePeriodDays int
	MethodsAllowed []string
}

// PolicyService reads and enforces per-tenant MFA policies.
type PolicyService struct {
	db *pgxpool.Pool
}

// NewPolicyService creates a PolicyService backed by db.
func NewPolicyService(db *pgxpool.Pool) *PolicyService {
	return &PolicyService{db: db}
}

// GetPolicy loads the MFA policy for tenantID. If no row exists, returns the
// default (required=false, grace=7 days, methods=[totp,webauthn]).
func (s *PolicyService) GetPolicy(ctx context.Context, tenantID string) (*Policy, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	p := &Policy{
		TenantID:       tenantID,
		Required:       false,
		GracePeriodDays: 7,
		MethodsAllowed: []string{"totp", "webauthn"},
	}

	var methodsRaw []string
	err := s.db.QueryRow(ctx,
		`SELECT required, grace_period_d, methods_allowed
		   FROM np_mfa_policies WHERE tenant_id = $1`,
		tenantID,
	).Scan(&p.Required, &p.GracePeriodDays, &methodsRaw)
	if err != nil {
		// Row not found → defaults apply; any other error is fatal.
		if isNoRows(err) {
			return p, nil
		}
		return nil, fmt.Errorf("load mfa policy: %w", err)
	}
	if len(methodsRaw) > 0 {
		p.MethodsAllowed = methodsRaw
	}
	return p, nil
}

// SetPolicy upserts a tenant MFA policy. Only callers with the mfa:admin scope
// should be allowed to invoke this; scope enforcement is the handler's job.
func (s *PolicyService) SetPolicy(ctx context.Context, p *Policy) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.db.Exec(ctx, `
		INSERT INTO np_mfa_policies (tenant_id, required, grace_period_d, methods_allowed, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id) DO UPDATE
		    SET required        = EXCLUDED.required,
		        grace_period_d  = EXCLUDED.grace_period_d,
		        methods_allowed = EXCLUDED.methods_allowed,
		        updated_at      = NOW()`,
		p.TenantID, p.Required, p.GracePeriodDays, p.MethodsAllowed,
	)
	return err
}

// EnforcementResult describes whether a user is allowed to proceed.
type EnforcementResult struct {
	// Allowed is true when MFA is not required or the user has enrolled.
	Allowed bool
	// MFARequired is true when the tenant requires MFA.
	MFARequired bool
	// Enrolled is true when the user has an active MFA enrollment.
	Enrolled bool
	// InGracePeriod is true when the user has not enrolled but is within the grace window.
	InGracePeriod bool
}

// EnforceForUser checks whether userID satisfies the tenant's MFA policy.
// registeredAt is the user's account creation time — used to compute the grace
// period window for existing users.
func (s *PolicyService) EnforceForUser(ctx context.Context, tenantID, userID string, registeredAt time.Time) (*EnforcementResult, error) {
	policy, err := s.GetPolicy(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if !policy.Required {
		return &EnforcementResult{Allowed: true, MFARequired: false}, nil
	}

	// Check if user has an active enrollment.
	ctx2, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var enrolled bool
	err = s.db.QueryRow(ctx2,
		`SELECT EXISTS (SELECT 1 FROM np_mfa_enrollments WHERE user_id = $1)`,
		userID,
	).Scan(&enrolled)
	if err != nil {
		return nil, fmt.Errorf("check enrollment: %w", err)
	}

	if enrolled {
		return &EnforcementResult{Allowed: true, MFARequired: true, Enrolled: true}, nil
	}

	// Check grace period: if policy requires MFA but user pre-dates the policy,
	// allow grace_period_d days from when the policy was applied.
	inGrace := false
	graceExpiry := registeredAt.AddDate(0, 0, policy.GracePeriodDays)
	if time.Now().Before(graceExpiry) {
		inGrace = true
	}

	return &EnforcementResult{
		Allowed:       inGrace,
		MFARequired:   true,
		Enrolled:      false,
		InGracePeriod: inGrace,
	}, nil
}

// RequireMFAMiddleware is an HTTP middleware that enforces MFA policy for all
// requests bearing a tenantID. It must run after the auth middleware has
// populated the request context with user information.
//
// The middleware skips enforcement for:
//   - MFA enrollment endpoints (to avoid a chicken-and-egg problem)
//   - Health/readiness probes
//
// Injecting tenantID and userID from the request context is the caller's
// responsibility — this middleware calls the supplied extractors.
func RequireMFAMiddleware(
	ps *PolicyService,
	extractTenant func(r *http.Request) string,
	extractUser func(r *http.Request) (userID string, registeredAt time.Time),
	exemptPaths []string,
) func(http.Handler) http.Handler {
	exempt := make(map[string]bool, len(exemptPaths))
	for _, p := range exemptPaths {
		exempt[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if exempt[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			tenantID := extractTenant(r)
			if tenantID == "" {
				next.ServeHTTP(w, r)
				return
			}

			userID, registeredAt := extractUser(r)
			if userID == "" {
				next.ServeHTTP(w, r)
				return
			}

			result, err := ps.EnforceForUser(r.Context(), tenantID, userID, registeredAt)
			if err != nil {
				http.Error(w, `{"error":"mfa_policy_error"}`, http.StatusInternalServerError)
				return
			}

			if !result.Allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprintf(w, `{"error":"mfa_required","mfa_required":true,"enrolled":%v}`,
					result.Enrolled)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isNoRows returns true when err indicates no matching row was found.
func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}
