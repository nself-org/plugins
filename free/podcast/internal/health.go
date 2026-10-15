package internal

import (
	"encoding/json"
	"net/http"
)

// Health is the liveness endpoint. bootMigrationsHealth answers 503 itself when the ledger cannot be read,
// so it runs before the body is written and the 200 below is skipped in that case.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mig := bootMigrationsHealth(r.Context(), w)
	body := map[string]any{"status": "ok", "plugin": "podcast", "migrations": mig}
	if mig["applied"] < 0 {
		body["status"] = "error"
		_ = json.NewEncoder(w).Encode(body)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}
