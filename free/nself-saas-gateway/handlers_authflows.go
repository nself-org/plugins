package main

// handlers_authflows.go — email verification + password reset.
//
// Purpose: the two token-by-email account flows. Tokens are 32-byte one-time
//   secrets stored only as SHA-256 (np_saas_email_tokens), single-use
//   (used_at), short-lived, and consumed atomically (UPDATE ... RETURNING).
// Routes (PUBLIC, per-IP rate-limited — no session exists yet):
//   POST /v1/verify-email          {"token"}            → sets tenants.verified
//   POST /v1/verify-email/resend   {"email"}            → 202 always
//   POST /v1/password/forgot       {"email"}            → 202 always
//   POST /v1/password/reset        {"token","password"} → sets password hash
// Constraints — ANTI-ENUMERATION: resend/forgot return the SAME 202 body
//   whether or not the email exists; invalid and expired tokens return the
//   same 400. Email sends are best-effort (logged, never surfaced as an
//   existence signal). Emails use the shared pkg templates verify-email /
//   password-reset.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/email"
)

// sha256Hex hashes a raw token for storage/lookup.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Token lifetimes.
const (
	verifyTokenTTL = 24 * time.Hour
	resetTokenTTL  = time.Hour
)

// createEmailToken mints and stores a one-time token for the tenant,
// returning the raw secret (for the email link).
func (g *gateway) createEmailToken(ctx context.Context, tenantID, kind string, ttl time.Duration) (string, error) {
	raw, hash, err := newInviteToken()
	if err != nil {
		return "", err
	}
	_, err = g.db.ExecContext(ctx, `
		INSERT INTO np_saas_email_tokens (token_hash, tenant_id, kind, expires_at)
		VALUES ($1, $2, $3, $4)`,
		hash, tenantID, kind, time.Now().UTC().Add(ttl))
	if err != nil {
		return "", err
	}
	return raw, nil
}

// consumeEmailToken atomically marks an unused, unexpired token used and
// returns its tenant. sql.ErrNoRows = invalid/expired/already-used.
func (g *gateway) consumeEmailToken(ctx context.Context, rawToken, kind string) (string, error) {
	hash := sha256Hex(rawToken)
	var tenantID string
	err := g.db.QueryRowContext(ctx, `
		UPDATE np_saas_email_tokens
		SET used_at = NOW()
		WHERE token_hash = $1 AND kind = $2 AND used_at IS NULL AND expires_at > NOW()
		RETURNING tenant_id::text`, hash, kind).Scan(&tenantID)
	return tenantID, err
}

// sendVerifyEmail mints a verify token and emails the verification link.
// Best-effort: returns whether the email went out.
func (g *gateway) sendVerifyEmail(ctx context.Context, tenantID, to, name string) bool {
	if g.mail == nil {
		return false
	}
	raw, err := g.createEmailToken(ctx, tenantID, "verify", verifyTokenTTL)
	if err != nil {
		log.Printf("saas-gateway: verify token for %s: %v", tenantID, err)
		return false
	}
	err = g.mail.Send(ctx, to, email.TemplateVerifyEmail, map[string]string{
		"Name":      firstNonEmpty(name, to),
		"VerifyURL": g.cfg.DashboardBaseURL + "/verify-email?token=" + raw,
	})
	if err != nil {
		log.Printf("saas-gateway: verify email to %s: %v", to, err)
		return false
	}
	return true
}

// decodeFlowBody decodes a small JSON body shared by the flow handlers.
func decodeFlowBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// handleVerifyEmail — POST /v1/verify-email {"token"}.
func (g *gateway) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "invalid or expired token")
		return
	}
	tenantID, err := g.consumeEmailToken(r.Context(), req.Token, "verify")
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusBadRequest, "invalid_token", "invalid or expired token")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "verification failed")
		return
	}
	if _, err := g.db.ExecContext(r.Context(),
		`UPDATE np_saas_tenants SET verified = true, updated_at = NOW() WHERE tenant_id = $1`,
		tenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "verification failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verified": true})
}

// handleResendVerifyEmail — POST /v1/verify-email/resend {"email"}.
// Always 202 (anti-enumeration).
func (g *gateway) handleResendVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email != "" {
		var tenantID string
		var name sql.NullString
		err := g.db.QueryRowContext(r.Context(), `
			SELECT tenant_id::text, owner_name FROM np_saas_tenants
			WHERE owner_email = $1 AND verified = false
			ORDER BY created_at DESC LIMIT 1`, req.Email).Scan(&tenantID, &name)
		if err == nil {
			g.sendVerifyEmail(r.Context(), tenantID, req.Email, name.String)
		} else if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("saas-gateway: resend verify lookup: %v", err)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"detail":   "if that address has an unverified account, a verification email is on its way",
	})
}

// handlePasswordForgot — POST /v1/password/forgot {"email"}.
// Always 202 (anti-enumeration). Only password-login tenants get a token.
func (g *gateway) handlePasswordForgot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email != "" && g.mail != nil {
		var tenantID string
		var name sql.NullString
		err := g.db.QueryRowContext(r.Context(), `
			SELECT tenant_id::text, owner_name FROM np_saas_tenants
			WHERE owner_email = $1 AND owner_password_hash IS NOT NULL
			LIMIT 1`, req.Email).Scan(&tenantID, &name)
		switch {
		case err == nil:
			if raw, terr := g.createEmailToken(r.Context(), tenantID, "reset", resetTokenTTL); terr == nil {
				if serr := g.mail.Send(r.Context(), req.Email, email.TemplatePasswordReset, map[string]string{
					"Name":     firstNonEmpty(name.String, req.Email),
					"ResetURL": g.cfg.DashboardBaseURL + "/reset-password?token=" + raw,
				}); serr != nil {
					log.Printf("saas-gateway: reset email to %s: %v", req.Email, serr)
				}
			} else {
				log.Printf("saas-gateway: reset token for %s: %v", tenantID, terr)
			}
		case !errors.Is(err, sql.ErrNoRows):
			log.Printf("saas-gateway: forgot lookup: %v", err)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"detail":   "if that address has an account, a reset email is on its way",
	})
}

// handlePasswordReset — POST /v1/password/reset {"token","password"}.
func (g *gateway) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", msg)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "invalid or expired token")
		return
	}
	tenantID, err := g.consumeEmailToken(r.Context(), req.Token, "reset")
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusBadRequest, "invalid_token", "invalid or expired token")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "password reset failed")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_error", "password hashing failed")
		return
	}
	if _, err := g.db.ExecContext(r.Context(), `
		UPDATE np_saas_tenants SET owner_password_hash = $1, updated_at = NOW()
		WHERE tenant_id = $2`, hash, tenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "password reset failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reset": true})
}
