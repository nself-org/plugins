// Package handlers wires all HIPAA sub-package handlers onto the chi router.
package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-hipaa/baa"
	"github.com/nself-org/nself-hipaa/encryption"
	"github.com/nself-org/nself-hipaa/phi"
)

// RegisterRoutes mounts all /hipaa/* routes on r.
//
// Purpose: Wire HIPAA PHI + BAA routes with license-gate middleware.
// Inputs: chi.Router + pgxpool.Pool.
// Outputs: Routes registered; 403 returned for any route when license flag is false.
// Constraints:
//   - ctx.License.Valid() equivalent: NSELF_HIPAA=true env var, checked via requireFlag.
//   - PHI endpoints blocked unless NSELF_HIPAA=true (ɳSelf+ license required).
//   - BAA endpoints additionally require NSELF_HIPAA_BAA=true.
//   - Health endpoint (/hipaa/health) always available without license check.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool) {
	hipaaEnabled := os.Getenv("NSELF_HIPAA") == "true"
	baaEnabled := os.Getenv("NSELF_HIPAA_BAA") == "true"

	r.Route("/hipaa", func(r chi.Router) {
		// Health — always available.
		r.Get("/health", healthHandler())

		// PHI column registry (ɳSelf+ — NSELF_HIPAA=true)
		r.With(requireFlag(hipaaEnabled, "NSELF_HIPAA")).Group(func(r chi.Router) {
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
		})

		// BAA workflow (Enterprise — NSELF_HIPAA_BAA=true)
		r.With(requireFlag(baaEnabled, "NSELF_HIPAA_BAA")).Group(func(r chi.Router) {
			r.Get("/baa", baa.GetBAAHandler(pool))
			r.Post("/baa/request", baa.RequestBAAHandler(pool))
			r.Post("/baa/activate", baa.ActivateBAAHandler(pool))
			r.Post("/baa/terminate", baa.TerminateBAAHandler(pool))
		})
	})
}

// requireFlag returns a middleware that returns 403 if the feature flag is disabled.
func requireFlag(enabled bool, flagName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": flagName + " is not enabled — ɳSelf+ license required",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
