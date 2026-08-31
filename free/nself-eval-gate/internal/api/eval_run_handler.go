package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/cache"
	"github.com/nself-org/nself-eval-gate/internal/db"
)

// HandleEvalRun handles POST /eval/run.
// Purpose: Accept a suite run request and dispatch it; returns run ID immediately.
// Inputs: JSON body {suite_slug, repo, commit_sha, branch, k}.
// Outputs: {run_id, status: "queued"} on accepted; error on invalid request.
// Constraints: Actual scoring runs asynchronously; GET /eval/runs/{id} for results.
type evalRunRequest struct {
	SuiteSlug string `json:"suite_slug"`
	Repo      string `json:"repo"`
	CommitSHA string `json:"commit_sha,omitempty"`
	Branch    string `json:"branch,omitempty"`
	K         int    `json:"k,omitempty"`
}

// HandleEvalRun returns a handler for POST /eval/run.
func HandleEvalRun(store db.Store, maxConcurrency int, judgeModel string,
	judgeTimeout, embedTimeout time.Duration, evalCache cache.EvalCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req evalRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.SuiteSlug == "" {
			writeError(w, http.StatusBadRequest, "suite_slug is required")
			return
		}

		sourceAcct := sourceAccountID(r)
		suite, err := store.GetSuiteBySlug(r.Context(), req.SuiteSlug, sourceAcct)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to look up suite")
			return
		}
		if suite == nil {
			writeError(w, http.StatusNotFound, "suite not found: "+req.SuiteSlug)
			return
		}

		// For P4, return queued status. Async execution wired in W3/S06/T01.
		writeJSON(w, http.StatusAccepted, map[string]string{
			"run_id": "pending",
			"status": "queued",
			"suite":  req.SuiteSlug,
		})
	}
}
