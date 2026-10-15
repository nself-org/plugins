package internal

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// registerHealth mounts GET and HEAD /health with the "migrations" ledger member (contract plugin.boot-migrations v1).
// bootMigrationsHealth answers 503 itself when the ledger cannot be read, so it runs before any header is written.
func registerHealth(r chi.Router) {
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mig := bootMigrationsHealth(req.Context(), w)
		body := map[string]any{"status": "ok", "plugin": "push", "migrations": mig}
		if mig["applied"] < 0 {
			body["status"] = "error"
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	r.Head("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mig := bootMigrationsHealth(req.Context(), w); mig["applied"] >= 0 {
			w.WriteHeader(http.StatusOK)
		}
	})
}
