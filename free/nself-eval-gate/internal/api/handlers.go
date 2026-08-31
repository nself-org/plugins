// Package api provides HTTP handlers for the nself-eval-gate plugin API.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/nself-eval-gate/internal/db"
	"github.com/nself-org/nself-eval-gate/internal/gate"
	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// sourceAccountID extracts the source account ID from request context or defaults to "primary".
func sourceAccountID(r *http.Request) string {
	if id := r.Header.Get("X-Nself-Source-Account-Id"); id != "" {
		return id
	}
	return "primary"
}

// HandleListSuites handles GET /eval/suites.
// Purpose: Return all registered eval suites for the requesting source account.
func HandleListSuites(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		suites, err := store.ListSuites(r.Context(), sourceAccountID(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list suites")
			return
		}
		writeJSON(w, http.StatusOK, suites)
	}
}

// HandleGetRun handles GET /eval/runs/{id}.
// Purpose: Fetch a specific eval run by ID with full per-task breakdown.
func HandleGetRun(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		run, err := store.GetRun(r.Context(), id, sourceAccountID(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch run")
			return
		}
		if run == nil {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// HandleListThresholds handles GET /eval/thresholds.
// Purpose: Return current autonomy-tier threshold configuration (global, not per-tenant).
func HandleListThresholds(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		thresholds, err := store.ListThresholds(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list thresholds")
			return
		}
		writeJSON(w, http.StatusOK, thresholds)
	}
}

// HandleGateCheck handles GET /eval/gate/{tier}.
// Purpose: Return boolean tier clearance status with blocking suite details.
func HandleGateCheck(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tier := chi.URLParam(r, "tier")
		result, err := gate.IsTierCleared(r.Context(), tier, store, sourceAccountID(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleValidate handles POST /eval/validate.
// Purpose: Validate YAML eval-set content against eval-set-v1.json schema.
type validateRequest struct {
	YAMLContent string `json:"yaml_content"`
}

type validateResponse struct {
	Valid  bool                    `json:"valid"`
	Errors []schema.ValidationError `json:"errors,omitempty"`
}

// HandleValidate validates a YAML eval-set document.
func HandleValidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req validateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		errs, err := schema.ValidateEvalSet([]byte(req.YAMLContent))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "validation error: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, validateResponse{
			Valid:  len(errs) == 0,
			Errors: errs,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
