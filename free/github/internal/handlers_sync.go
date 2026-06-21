package internal

import (
	"encoding/json"
	"net/http"
	"time"
)

// --- Sync endpoint -----------------------------------------------------------

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	var since *time.Time
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid since format, expected RFC3339"})
			return
		}
		since = &t
	}

	ctx := r.Context()

	var result *SyncResult
	if len(req.Resources) == 0 {
		result = s.syncService.SyncAll(ctx)
	} else {
		// Sync each requested resource and merge results
		merged := &SyncResult{Success: true}
		for _, res := range req.Resources {
			partial := s.syncService.SyncResource(ctx, res)
			merged.Errors = append(merged.Errors, partial.Errors...)
			merged.Duration += partial.Duration
			merged.Stats = partial.Stats
		}
		merged.Success = len(merged.Errors) == 0
		result = merged
	}
	_ = since // reserved for future incremental sync support

	writeJSON(w, http.StatusOK, result)
}

