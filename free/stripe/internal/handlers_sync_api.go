package internal

import (
	"net/http"
)


// ============================================================================
// Sync
// ============================================================================

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if s.Client == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Stripe client not configured"})
		return
	}

	results := SyncAll(r.Context(), s.DB, s.Client, s.Accounts)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if s.Client == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Stripe client not configured"})
		return
	}

	results := Reconcile(r.Context(), s.DB, s.Client, s.Accounts)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}

// ============================================================================
// API Endpoints
// ============================================================================
