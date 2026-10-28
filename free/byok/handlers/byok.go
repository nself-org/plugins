package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-byok/tenantdb"
)

// HealthHandler returns 200 OK with status information.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// The migrations callback owns the 503 status on failure; encoding supplies 200 otherwise.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok", "plugin": "byok", "version": "0.1.0",
		"migrations": bootMigrationsHealth(w),
	})
}

// ReadyHandler checks the DB connection before returning 200.
func ReadyHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// License gate — all endpoints require Enterprise
// ──────────────────────────────────────────────────────────────────────────

// enterpriseGate returns a 403 if NSELF_BYOK is not "true".
// This middleware must wrap every route in the BYOK plugin.
func enterpriseGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !byokEnabled() {
			errJSON(w, http.StatusForbidden, "BYOK requires an Enterprise license. Set NSELF_BYOK=true after purchasing.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func byokEnabled() bool {
	return strings.ToLower(os.Getenv("NSELF_BYOK")) == "true"
}

// ──────────────────────────────────────────────────────────────────────────
// Router
// ──────────────────────────────────────────────────────────────────────────

// RegisterRoutes registers all BYOK HTTP routes on r.
func RegisterRoutes(r chi.Router, db *pgxpool.Pool) {
	h := &byokHandler{db: db}

	r.Get("/health", HealthHandler)
	r.Get("/ready", ReadyHandler(db))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(enterpriseGate)

		// KMS configuration
		r.Get("/encryption/kms", h.GetKMSConfig)
		r.Post("/encryption/kms", h.ConfigureKMS)
		r.Put("/encryption/kms", h.UpdateKMSConfig)
		r.Post("/encryption/kms/verify", h.VerifyKMS)

		// Key rotation
		r.Post("/encryption/rotate", h.TriggerRotation)
		r.Get("/encryption/rotate/{job_id}", h.RotationStatus)
		r.Get("/encryption/key-events", h.ListKeyEvents)
	})
}

// ──────────────────────────────────────────────────────────────────────────
// Handler struct
// ──────────────────────────────────────────────────────────────────────────

// byokHandler reaches the database only through tenantdb.With, so every statement runs
// under the requesting tenant's row-level security (D-0956).
type byokHandler struct {
	db tenantdb.Beginner
}
