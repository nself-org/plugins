package internal

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler holds the HTTP handlers for the mlflow management proxy.
type Handler struct {
	client     *MLflowClient
	mlflowURL  string
}

// NewHandler creates a Handler backed by the given MLflowClient.
func NewHandler(client *MLflowClient) *Handler {
	return &Handler{
		client:    client,
		mlflowURL: client.BaseURL,
	}
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --------------------------------------------------------------------------
// GET /health
// --------------------------------------------------------------------------

// Health returns a liveness check for this management service.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "healthy",
		"service":     "nself-mlflow",
		"mlflow_url":  h.mlflowURL,
	})
}

// --------------------------------------------------------------------------
// GET /v1/experiments
// --------------------------------------------------------------------------

// ListExperiments proxies experiment search to the MLflow server.
func (h *Handler) ListExperiments(w http.ResponseWriter, r *http.Request) {
	experiments, err := h.client.SearchExperiments(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to list experiments: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"experiments": experiments,
	})
}

// --------------------------------------------------------------------------
// GET /v1/experiments/{id}
// --------------------------------------------------------------------------

// GetExperiment proxies a single experiment lookup to the MLflow server.
func (h *Handler) GetExperiment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	experiment, err := h.client.GetExperiment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to get experiment: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"experiment": experiment,
	})
}

// --------------------------------------------------------------------------
// GET /v1/runs?experiment_id=X
// --------------------------------------------------------------------------

// ListRuns proxies run search for a given experiment to the MLflow server.
func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	experimentID := r.URL.Query().Get("experiment_id")
	if experimentID == "" {
		writeError(w, http.StatusBadRequest, "experiment_id query parameter is required")
		return
	}
	runs, err := h.client.SearchRuns(r.Context(), experimentID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to list runs: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runs": runs,
	})
}

// --------------------------------------------------------------------------
// GET /v1/runs/{run_id}
// --------------------------------------------------------------------------

// GetRun proxies a single run lookup to the MLflow server.
func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	run, err := h.client.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to get run: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"run": run,
	})
}

// --------------------------------------------------------------------------
// GET /v1/models
// --------------------------------------------------------------------------

// ListModels proxies registered model search to the MLflow model registry.
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.client.SearchRegisteredModels(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to list models: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models": models,
	})
}

// --------------------------------------------------------------------------
// GET /v1/models/{name}
// --------------------------------------------------------------------------

// GetModel proxies a single registered model lookup to the MLflow model registry.
func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	model, err := h.client.GetRegisteredModel(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to get model: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"model": model,
	})
}

// --------------------------------------------------------------------------
// GET /v1/metrics
// --------------------------------------------------------------------------

// metricsResponse holds aggregate counts across experiments, runs, and models.
type metricsResponse struct {
	TotalExperiments int `json:"total_experiments"`
	TotalRuns        int `json:"total_runs"`
	TotalModels      int `json:"total_models"`
}

// GetMetrics aggregates counts across the MLflow server.
func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	experiments, err := h.client.SearchExperiments(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch experiments for metrics: "+err.Error())
		return
	}

	totalRuns := 0
	for _, exp := range experiments {
		runs, err := h.client.SearchRuns(ctx, exp.ExperimentID)
		if err != nil {
			// Continue accumulating what we can; a single failed experiment
			// should not abort the entire metrics response.
			continue
		}
		totalRuns += len(runs)
	}

	models, err := h.client.SearchRegisteredModels(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch models for metrics: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, metricsResponse{
		TotalExperiments: len(experiments),
		TotalRuns:        totalRuns,
		TotalModels:      len(models),
	})
}
