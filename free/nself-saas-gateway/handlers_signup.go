package main

// handlers_signup.go — POST /v1/signup + POST /internal/billing/tenant-tier.
//
// Purpose: the two tenant-lifecycle entry points.
//   /v1/signup (public, unauthenticated): create tenant + owner email +
//     first full-scope API key (raw key shown exactly once). The SPA (W3)
//     calls this; the free tier needs no billing step.
//   /internal/billing/tenant-tier (shared secret NSENTRY_INTERNAL_API_KEY):
//     W4's Stripe webhook handler upserts the tenant's tier/customer id
//     after checkout/cancellation events.
// Constraints: the internal endpoint is fail-CLOSED (503 when the secret is
//   unconfigured, 401 on mismatch, constant-time compare). Signup rate
//   limiting is delegated to the fronting nginx (W2 box config).

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// handleSignup — POST /v1/signup {"email", "password"?, "name"?} → 201
// tenant + dev key. When a password is supplied (SPA path) the bcrypt hash
// is stored for /v1/login and the response additionally carries a session
// JWT ("token") so the browser lands signed-in. Passwordless signup (the
// original CLI/nsk path) is byte-for-byte unchanged.
func (g *gateway) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "a valid email is required")
		return
	}

	var passwordHash string
	if req.Password != "" {
		if msg := validatePassword(req.Password); msg != "" {
			writeErr(w, http.StatusUnprocessableEntity, "invalid_request", msg)
			return
		}
		if len(g.cfg.JWTSecret) == 0 {
			writeErr(w, http.StatusServiceUnavailable, "auth_disabled",
				"SAAS_JWT_HS256_SECRET is not configured")
			return
		}
		var err error
		if passwordHash, err = hashPassword(req.Password); err != nil {
			writeErr(w, http.StatusInternalServerError, "hash_error", "password hashing failed")
			return
		}
	}

	tenantID := uuid.NewString()
	if _, err := g.db.ExecContext(r.Context(), `
		INSERT INTO np_saas_tenants (tenant_id, tier, owner_email, owner_name, owner_password_hash)
		VALUES ($1, 'free', $2, NULLIF($3, ''), NULLIF($4, ''))`,
		tenantID, req.Email, req.Name, passwordHash); err != nil {
		// Unique partial index idx_np_saas_tenants_owner_email_login: one
		// password login per email.
		if strings.Contains(err.Error(), "idx_np_saas_tenants_owner_email_login") ||
			strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "email_taken",
				"an account with this email already exists — sign in instead")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant create failed")
		return
	}

	// Full-scope key: the middleware clamps free-tier keys to read-only at
	// request time, and the key upgrades transparently with the tenant.
	rawKey, _, err := saas.CreateAPIKey(r.Context(), g.db, tenantID, "default", saas.ScopeFull)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "api key create failed")
		return
	}

	// Email verification: mint a one-time token and send the verify link
	// (handlers_authflows.go). Best-effort — signup never fails on SMTP;
	// /v1/verify-email/resend covers misses.
	verifySent := g.sendVerifyEmail(r.Context(), tenantID, req.Email, req.Name)

	resp := map[string]any{
		"tenant_id":         tenantID,
		"email":             req.Email,
		"tier":              saas.TierFree,
		"api_key":           rawKey, // shown exactly once — only the hash is stored
		"verify_email_sent": verifySent,
	}
	if passwordHash != "" {
		if token, err := g.issueSession(tenantID, req.Email, req.Name, saas.TierFree); err == nil {
			resp["token"] = token
			resp["expires_in"] = int64(sessionTTL.Seconds())
		}
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleTenantTier — POST /internal/billing/tenant-tier.
// Body: {"tenant_id","tier","stripe_customer_id"?,"owner_email"?}.
func (g *gateway) handleTenantTier(w http.ResponseWriter, r *http.Request) {
	// Re-exposed externally (nginx) ONLY for the prod ping_api box → gate on
	// the source-IP allowlist first, then the constant-time internal key.
	if !g.internalIPAllowed(w, r) {
		return
	}
	if !g.internalAuthOK(w, r) {
		return
	}

	var req struct {
		TenantID         string `json:"tenant_id"`
		Tier             string `json:"tier"`
		StripeCustomerID string `json:"stripe_customer_id"`
		OwnerEmail       string `json:"owner_email"`
		Email            string `json:"email"` // ping_api client alias for owner_email
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	if uuid.Validate(req.TenantID) != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "tenant_id must be a UUID")
		return
	}
	email := req.OwnerEmail
	if email == "" {
		email = req.Email
	}

	// Normalize SaaS-plan tier aliases onto the canonical np_saas_tenants
	// values. The ping_api entitlement layer emits 'sentry' (and 'product')
	// for the single paid bundle; the tenant table + saas.LimitsFor use
	// 'bundle'. Without this map a real bundle purchase would 422.
	tier := normalizeTier(req.Tier)
	switch saas.Tier(tier) {
	case saas.TierFree, saas.TierBundle, saas.TierPlus:
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"tier must be free, sentry/bundle, or plus")
		return
	}

	// Upsert: webhook events may arrive for tenants created out-of-band.
	_, err := g.db.ExecContext(r.Context(), `
		INSERT INTO np_saas_tenants (tenant_id, tier, stripe_customer_id, owner_email)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))
		ON CONFLICT (tenant_id) DO UPDATE SET
			tier               = EXCLUDED.tier,
			stripe_customer_id = COALESCE(EXCLUDED.stripe_customer_id, np_saas_tenants.stripe_customer_id),
			owner_email        = COALESCE(EXCLUDED.owner_email, np_saas_tenants.owner_email),
			updated_at         = NOW()`,
		req.TenantID, tier, req.StripeCustomerID, email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant tier update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": req.TenantID,
		"tier":      tier,
		"updated":   true,
	})
}

// normalizeTier maps SaaS-plan / entitlement tier aliases onto the canonical
// np_saas_tenants tier strings ('free' | 'bundle' | 'plus').
func normalizeTier(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "sentry", "product", "bundle":
		return string(saas.TierBundle)
	case "plus":
		return string(saas.TierPlus)
	case "free", "":
		return string(saas.TierFree)
	default:
		return strings.ToLower(strings.TrimSpace(t))
	}
}
