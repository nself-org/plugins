// Purpose: the /health handler, split out of handlers.go to keep that file under its size budget.
// Outputs: JSON status with the boot-migration counts.
package internal

import (
	"net/http"
	"time"
)

func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	// bootMigrationsHealth answers 503 itself when the migration status cannot be read, so it runs first
	// and the content type is set before that header goes out.
	w.Header().Set("Content-Type", "application/json")
	migrations := bootMigrationsHealth(r.Context(), w)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"plugin":     "content-safety",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"migrations": migrations,
	})
}
