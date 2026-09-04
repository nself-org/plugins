// Package handlers wires all HIPAA sub-package handlers onto the chi router.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-hipaa/baa"
	"github.com/nself-org/nself-hipaa/encryption"
	"github.com/nself-org/nself-hipaa/phi"
)

// RegisterRoutes mounts all /hipaa/* routes on r.
//
// Purpose: Wire HIPAA PHI + BAA routes.
// Inputs: chi.Router + pgxpool.Pool.
// Outputs: Routes registered.
// Constraints:
//   - hipaa is a free plugin (plugin.json: requires_license=false,
//     requiredEntitlements=[]) — the former NSELF_HIPAA / NSELF_HIPAA_BAA
//     env-var license-gate middleware (requireFlag, 403 "ɳSelf+ license
//     required") was removed 2026-09-03 (P6-E3-W2-S1-T5 FIX-PLUGINS) since
//     the manifest documents no gated entitlements to keep it for.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool) {
	r.Route("/hipaa", func(r chi.Router) {
		// Health — always available.
		r.Get("/health", healthHandler())

		// PHI column registry
		r.Get("/phi-columns", phi.ListPHIColumnsHandler(pool))
		r.Post("/phi-columns", phi.RegisterPHIColumnHandler(pool))
		r.Delete("/phi-columns/{id}", phi.UnregisterPHIColumnHandler(pool))

		// PHI audit log
		r.Get("/audit-log", phi.ListAuditLogHandler(pool))
		r.Get("/audit-log/export", phi.ExportAuditLogHandler(pool))

		// De-identification + tokenization
		r.Post("/deidentify", phi.DeidentifyHandler(pool))
		r.Post("/tokenize", phi.TokenizeHandler(pool))
		r.Get("/tokenize/{token}", phi.DetokenizeHandler(pool))

		// Encryption audit
		r.Get("/encryption-audit", encryption.AuditEncryptionHandler(pool))

		// BAA workflow
		r.Get("/baa", baa.GetBAAHandler(pool))
		r.Post("/baa/request", baa.RequestBAAHandler(pool))
		r.Post("/baa/activate", baa.ActivateBAAHandler(pool))
		r.Post("/baa/terminate", baa.TerminateBAAHandler(pool))
	})
}

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "ok",
			"plugin":    "hipaa",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}
