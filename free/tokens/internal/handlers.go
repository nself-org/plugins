package internal

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all tokens API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB, cfg *Config) {
	r.Route("/v1", func(r chi.Router) {
		// Token issuance and validation
		r.Post("/tokens/issue", handleIssueToken(db, cfg))
		r.Post("/tokens/validate", handleValidateToken(db, cfg))
		r.Post("/tokens/revoke", handleRevokeToken(db))
		r.Get("/tokens", handleListTokens(db))

		// Signing keys
		r.Get("/keys", handleListKeys(db))
		r.Post("/keys", handleCreateKey(db, cfg))
		r.Post("/keys/rotate", handleRotateKey(db, cfg))
		r.Delete("/keys/{id}", handleDeleteKey(db))

		// Entitlements
		r.Get("/entitlements", handleListEntitlements(db))
		r.Post("/entitlements", handleCreateEntitlement(db))
		r.Get("/entitlements/{id}", handleGetEntitlement(db))
		r.Delete("/entitlements/{id}", handleDeleteEntitlement(db))
		r.Post("/entitlements/check", handleCheckEntitlement(db, cfg))

		// HLS encryption keys
		r.Get("/encryption/keys", handleListEncryptionKeys(db))
		r.Post("/encryption/keys", handleCreateEncryptionKey(db, cfg))
		r.Get("/encryption/keys/{id}/deliver", handleDeliverEncryptionKey(db, cfg))
		r.Post("/encryption/keys/{contentId}/rotate", handleRotateEncryptionKey(db, cfg))

		// Stats
		r.Get("/stats", handleStats(db))
	})
}

// getSourceAccountID extracts the source account from the request.
// Delegates to sdk.SourceAccountID for DRY cross-plugin consistency.
func getSourceAccountID(r *http.Request) string {
	return sdk.SourceAccountID(r)
}

// ============================================================================
