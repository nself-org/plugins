package handlers

import (
	"net/http"
	"os"
)

// sharedSecretMiddleware validates the X-Audit-Analytics-Secret header against
// AUDIT_ANALYTICS_SHARED_SECRET. All analytics endpoints are internal
// (backend-to-backend only) — they must never be reached by unauthenticated
// or user-facing callers. The shared secret is set in the nSelf env cascade.
//
// Behaviour when AUDIT_ANALYTICS_SHARED_SECRET is empty:
//   - Cloud mode (NSELF_DEPLOY_MODE=cloud): fail-CLOSED — always returns 401.
//     An unconfigured shared secret in Cloud is a misconfiguration; open access
//     would expose cross-tenant analytics data. (S40-fix, CR-C gate 2026-05-17)
//   - Self-host mode: open-dev passthrough (already warned at startup via main.go
//     warnIfNoSecret). Self-host deployments are single-tenant; the risk is
//     operator-local only.
func sharedSecretMiddleware(secret string, next http.HandlerFunc) http.HandlerFunc {
	if secret == "" {
		if os.Getenv("NSELF_DEPLOY_MODE") == "cloud" {
			// Cloud + no secret = misconfiguration. Fail-closed to prevent
			// unauthenticated access to multi-tenant analytics endpoints.
			return func(w http.ResponseWriter, r *http.Request) {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
			}
		}
		// Self-host dev mode: no-op wrapper, already warned at startup.
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Audit-Analytics-Secret") != secret {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// SharedSecret returns the configured AUDIT_ANALYTICS_SHARED_SECRET value.
func SharedSecret() string {
	return os.Getenv("AUDIT_ANALYTICS_SHARED_SECRET")
}

// tenantIDFromRequest extracts the X-Audit-Tenant-Id request header.
// This header is set by the nSelf backend after validating the caller's JWT.
// Empty string means self-host (no multi-tenancy).
func tenantIDFromRequest(r *http.Request) string {
	return r.Header.Get("X-Audit-Tenant-Id")
}
