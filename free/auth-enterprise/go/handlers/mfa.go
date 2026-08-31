// Package handlers — mfa.go
//
// HTTP handlers for MFA enrollment, challenge, policy, and recovery endpoints.
//
// Endpoint map:
//   GET  /auth/mfa/status               — enrollment status for the current user
//   POST /auth/mfa/totp/setup           — begin TOTP enrollment
//   POST /auth/mfa/totp/verify          — confirm enrollment with a live code
//   POST /auth/mfa/totp/challenge       — verify TOTP code during login
//   POST /auth/mfa/recovery             — consume a recovery code
//   GET  /auth/mfa/recovery/codes       — list masked recovery codes
//   POST /auth/mfa/recovery/regenerate  — replace all recovery codes
//   GET  /auth/mfa/policy               — get tenant MFA policy
//   PUT  /auth/mfa/policy               — set tenant MFA policy (mfa:admin scope)
//
// WebAuthn endpoints are in the existing plugins-pro/paid/auth plugin and are
// not duplicated here. The auth-enterprise plugin extends that plugin with
// policy enforcement and SSO — it does not replace the WebAuthn handlers.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nself-org/nself-auth-enterprise/go/mfa"
)

// MFAHandlers groups all MFA HTTP handlers.
type MFAHandlers struct {
	totp     *mfa.TOTPService
	policy   *mfa.PolicyService
	recovery *mfa.RecoveryService
	throttle *mfa.Throttle
}

// NewMFAHandlers constructs an MFAHandlers from the provided services.
func NewMFAHandlers(totp *mfa.TOTPService, policy *mfa.PolicyService, recovery *mfa.RecoveryService) *MFAHandlers {
	return &MFAHandlers{
		totp:     totp,
		policy:   policy,
		recovery: recovery,
		throttle: mfa.NewThrottle(5, 5*time.Minute, 5*time.Minute),
	}
}

// ─── Status ───────────────────────────────────────────────────────────────────

// Status returns the MFA enrollment status for the authenticated user.
func (h *MFAHandlers) Status(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	count, err := h.recovery.CountUnused(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":                userID,
		"totp_enrolled":          count > 0,
		"recovery_codes_remaining": count,
	})
}

// ─── TOTP Setup ───────────────────────────────────────────────────────────────

// TOTPSetup starts a new TOTP enrollment for the authenticated user.
func (h *MFAHandlers) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		AccountName string `json:"account_name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.AccountName == "" {
		req.AccountName = userID
	}

	result, err := h.totp.BeginEnrollment(r.Context(), userID, req.AccountName, tenantIDFromContext(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── TOTP Verify (confirm enrollment) ────────────────────────────────────────

// TOTPVerify confirms a pending TOTP enrollment by validating a live code.
func (h *MFAHandlers) TOTPVerify(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	if h.throttle.IsLocked(userID) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": "too_many_attempts",
			"message": "too many failed attempts — try again later",
		})
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	ok, err := h.totp.ConfirmEnrollment(r.Context(), userID, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.throttle.RecordFailure(userID)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"verified": false,
			"error":    "invalid_code",
		})
		return
	}
	h.throttle.Reset(userID)
	writeJSON(w, http.StatusOK, map[string]any{"verified": true, "user_id": userID})
}

// ─── TOTP Challenge (during login) ───────────────────────────────────────────

// TOTPChallenge verifies a TOTP code submitted during the login flow.
func (h *MFAHandlers) TOTPChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "user_id and code are required")
		return
	}

	if h.throttle.IsLocked(req.UserID) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"authenticated": false,
			"error":         "too_many_attempts",
		})
		return
	}

	ok, err := h.totp.VerifyChallenge(r.Context(), req.UserID, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		h.throttle.RecordFailure(req.UserID)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"authenticated": false,
			"error":         "invalid_code",
		})
		return
	}
	h.throttle.Reset(req.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user_id": req.UserID})
}

// ─── Recovery code consumption ────────────────────────────────────────────────

// UseRecoveryCode validates and consumes one recovery code.
func (h *MFAHandlers) UseRecoveryCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "user_id and code are required")
		return
	}

	ok, err := h.totp.UseRecoveryCode(r.Context(), req.UserID, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"used":  false,
			"error": "invalid_or_used_code",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"used": true, "user_id": req.UserID})
}

// ─── Recovery code listing ────────────────────────────────────────────────────

// ListRecoveryCodes returns masked recovery codes for the authenticated user.
func (h *MFAHandlers) ListRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	codes, err := h.recovery.ListCodes(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": userID,
		"codes":   codes,
	})
}

// RegenerateCodes replaces all recovery codes for the authenticated user.
func (h *MFAHandlers) RegenerateCodes(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	newCodes, err := h.totp.RegenerateCodes(r.Context(), userID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": userID,
		"codes":   newCodes,
		"count":   len(newCodes),
	})
}

// ─── MFA Policy ───────────────────────────────────────────────────────────────

// GetPolicy returns the MFA policy for the caller's tenant.
func (h *MFAHandlers) GetPolicy(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	p, err := h.policy.GetPolicy(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// SetPolicy updates the MFA policy for the caller's tenant.
// Requires mfa:admin scope — scope enforcement is done by the router middleware.
func (h *MFAHandlers) SetPolicy(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	var input struct {
		Required        bool     `json:"required"`
		GracePeriodDays int      `json:"grace_period_d"`
		MethodsAllowed  []string `json:"methods_allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.GracePeriodDays < 0 {
		input.GracePeriodDays = 0
	}

	p := &mfa.Policy{
		TenantID:        tenantID,
		Required:        input.Required,
		GracePeriodDays: input.GracePeriodDays,
		MethodsAllowed:  input.MethodsAllowed,
	}
	if err := h.policy.SetPolicy(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ─── Context helpers ──────────────────────────────────────────────────────────

type contextKey string

const (
	ctxKeyUserID   contextKey = "user_id"
	ctxKeyTenantID contextKey = "tenant_id"
)

func userIDFromContext(r *http.Request) string {
	v, _ := r.Context().Value(ctxKeyUserID).(string)
	return v
}

func tenantIDFromContext(r *http.Request) string {
	v, _ := r.Context().Value(ctxKeyTenantID).(string)
	return v
}

// ─── Response helpers ─────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
