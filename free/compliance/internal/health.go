package internal

import (
	"net/http"
	"time"
)

// HealthHandler reports liveness plus the boot-migration counts (503 when the ledger cannot be read).
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	mig, code := bootMigrationsHealth()
	writeJSON(w, code, map[string]any{
		"status":     "ok",
		"plugin":     "compliance",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"migrations": mig,
	})
}
