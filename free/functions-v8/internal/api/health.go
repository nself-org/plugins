package api

import (
	"encoding/json"
	"net/http"

	"github.com/nself-org/nself-functions-v8/internal/invoke"
)

// Health handles GET /health — also reports pool state for nself doctor and, when wired, the boot-migrations
// ledger counts (the Migrations callback writes a 503 header itself when the ledger cannot be read).
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ps := h.pool.PoolSize()
	invoke.RecordPoolState(ps, ps) // best-effort gauge; real available count needs semaphore introspection
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"status": "ok", "pool_size": ps}
	if h.Migrations != nil {
		body["migrations"] = h.Migrations(w)
	}
	_ = json.NewEncoder(w).Encode(body)
}
