package internal

import (
	"encoding/json"
	"net/http"
)

// HandleHealth is the liveness endpoint. bootMigrationsHealth answers 503 itself when the ledger cannot be read,
// so it runs before the body is written and no second WriteHeader happens in that case.
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mig := bootMigrationsHealth(w)
	body := map[string]any{"status": "ok", "service": "nself-post", "migrations": mig}
	if mig["applied"] < 0 {
		body["status"] = "error"
		_ = json.NewEncoder(w).Encode(body)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}
