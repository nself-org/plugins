package main

// handlers_auth.go — password auth for the SaaS dashboard (W3 seam).
//
// Purpose: make the gateway the ɳSentry SaaS auth AUTHORITY — no separate
//   auth service. Three surfaces:
//     POST /v1/login   {email,password} → HS256 JWT (claims: tenant_id,
//       email, tier, name; exp 7d) signed with cfg.JWTSecret
//       (SAAS_JWT_HS256_SECRET). The SPA's Vercel functions store it in the
//       nsentry_auth_token cookie and verify it with the SAME secret.
//     GET  /v1/session  Bearer <jwt> → verified identity echo for clients
//       that cannot verify locally.
//     Password storage: bcrypt hash on np_saas_tenants.owner_password_hash
//       (set by /v1/signup when a password is supplied — handlers_signup.go).
// Constraints: nsk_ API-key auth (CLI/MCP) is untouched. Unknown-email and
//   wrong-password both return the same 401 (no account enumeration); a
//   dummy bcrypt compare keeps timing constant-ish. Login/signup are
//   rate-limited per IP (ratelimit.go) on top of the fronting nginx.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// sessionTTL is the login-token lifetime — matches the SPA cookie Max-Age.
const sessionTTL = 7 * 24 * time.Hour

// bcryptCost — default (10) balances login latency vs brute-force cost.
const bcryptCost = bcrypt.DefaultCost

// dummyHash is compared against when the email has no row, so unknown-email
// and wrong-password take comparable time (anti-enumeration).
var dummyHash = []byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0aZ8mM3o1a3lJ9jXqfKq0GJ9y9u")

// hashPassword bcrypt-hashes a password (72-byte bcrypt input cap enforced
// by validatePassword before this is called).
func hashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(h), err
}

// validatePassword enforces the signup password policy.
func validatePassword(pw string) string {
	if len(pw) < 8 {
		return "password must be at least 8 characters"
	}
	if len(pw) > 72 {
		return "password must be at most 72 characters"
	}
	return ""
}

// sessionClaims builds the JWT claim set for a tenant login. tenant_id is
// present both top-level and in the Hasura namespace so the token works as
// a bearer credential against the shared saas middleware too.
func sessionClaims(tenantID, email, name string, tier saas.Tier, now time.Time) map[string]any {
	return map[string]any{
		"iss":       "nself-saas-gateway",
		"sub":       tenantID,
		"tenant_id": tenantID,
		"email":     email,
		"name":      name,
		"tier":      string(tier),
		"iat":       now.Unix(),
		"exp":       now.Add(sessionTTL).Unix(),
		"https://hasura.io/jwt/claims": map[string]any{
			"x-hasura-tenant-id":     tenantID,
			"x-hasura-user-id":       tenantID,
			"x-hasura-default-role":  "user",
			"x-hasura-allowed-roles": []string{"user"},
		},
	}
}

// issueSession signs a session JWT for the tenant.
func (g *gateway) issueSession(tenantID, email, name string, tier saas.Tier) (string, error) {
	return saas.SignHS256(sessionClaims(tenantID, email, name, tier, time.Now().UTC()), g.cfg.JWTSecret)
}

// handleLogin — POST /v1/login {"email","password"} → {token, tenant_id,
// email, tier, name, expires_in}.
func (g *gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	if len(g.cfg.JWTSecret) == 0 {
		writeErr(w, http.StatusServiceUnavailable, "auth_disabled",
			"SAAS_JWT_HS256_SECRET is not configured")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "email and password are required")
		return
	}

	var (
		tenantID, hash string
		tier           string
		name           sql.NullString
	)
	err := g.db.QueryRowContext(r.Context(), `
		SELECT tenant_id, tier, owner_name, owner_password_hash
		FROM np_saas_tenants
		WHERE owner_email = $1 AND owner_password_hash IS NOT NULL
		LIMIT 1`, req.Email).Scan(&tenantID, &tier, &name, &hash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password)) // constant-time-ish
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "db_error", "login lookup failed")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}

	token, err := g.issueSession(tenantID, req.Email, name.String, saas.Tier(tier))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token_error", "session token signing failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"tenant_id":  tenantID,
		"email":      req.Email,
		"tier":       tier,
		"name":       name.String,
		"expires_in": int64(sessionTTL.Seconds()),
	})
}

// handleSession — GET /v1/session with Bearer <jwt> → verified identity.
func (g *gateway) handleSession(w http.ResponseWriter, r *http.Request) {
	if len(g.cfg.JWTSecret) == 0 {
		writeErr(w, http.StatusServiceUnavailable, "auth_disabled",
			"SAAS_JWT_HS256_SECRET is not configured")
		return
	}
	cred := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if cred == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}
	claims, err := saas.VerifyHS256Claims(cred, g.cfg.JWTSecret)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return
	}
	str := func(key string) string {
		var s string
		if raw, ok := claims[key]; ok {
			_ = json.Unmarshal(raw, &s)
		}
		return s
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"tenant_id":     str("tenant_id"),
		"email":         str("email"),
		"tier":          str("tier"),
		"name":          str("name"),
	})
}
