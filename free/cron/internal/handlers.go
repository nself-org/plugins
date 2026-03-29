package internal

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all cron API routes onto the given router.
func RegisterRoutes(r chi.Router, app *App) {
	r.Route("/v1/jobs", func(r chi.Router) {
		r.Post("/", handleCreateJob(app))
		r.Get("/", handleListJobs(app))
		r.Get("/{id}", handleGetJob(app))
		r.Put("/{id}", handleUpdateJob(app))
		r.Delete("/{id}", handleDeleteJob(app))
		r.Post("/{id}/trigger", handleTriggerJob(app))
	})
}

// --- Request/Response types ---

type createJobRequest struct {
	Name        string  `json:"name"`
	CronExpr    string  `json:"cron_expr"`
	CallbackURL string  `json:"callback_url"`
	Payload     *string `json:"payload,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

type updateJobRequest struct {
	Name        *string `json:"name,omitempty"`
	CronExpr    *string `json:"cron_expr,omitempty"`
	CallbackURL *string `json:"callback_url,omitempty"`
	Payload     *string `json:"payload,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	MaxAttempts *int    `json:"max_attempts,omitempty"`
}

type paginatedResponse struct {
	Data   interface{} `json:"data"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// --- Handlers ---

func handleCreateJob(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		if req.Name == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if req.CronExpr == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "cron_expr is required"})
			return
		}
		if req.CallbackURL == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "callback_url is required"})
			return
		}

		if err := ValidateCronExpr(req.CronExpr); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		job, err := CreateJob(app.Pool, req.Name, req.CronExpr, req.CallbackURL, req.Payload, enabled)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, job)
	}
}

func handleListJobs(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := ListJobs(app.Pool)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if jobs == nil {
			jobs = []CronJob{}
		}
		sdk.Respond(w, http.StatusOK, jobs)
	}
}

func handleGetJob(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := GetJob(app.Pool, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "job not found"})
				return
			}
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Fetch run history for this job.
		limitStr := r.URL.Query().Get("history_limit")
		limit := 20
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		runs, total, err := GetJobRuns(app.Pool, id, limit, 0)
		if err != nil {
			// Return job without history if history query fails.
			sdk.Respond(w, http.StatusOK, job)
			return
		}
		if runs == nil {
			runs = []CronRun{}
		}

		type jobWithHistory struct {
			*CronJob
			History      []CronRun `json:"history"`
			HistoryTotal int64     `json:"history_total"`
		}

		sdk.Respond(w, http.StatusOK, jobWithHistory{
			CronJob:      job,
			History:      runs,
			HistoryTotal: total,
		})
	}
}

func handleUpdateJob(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req updateJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		if req.CronExpr != nil {
			if err := ValidateCronExpr(*req.CronExpr); err != nil {
				sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}

		job, err := UpdateJob(app.Pool, id, req.Name, req.CronExpr, req.CallbackURL, req.Payload, req.Enabled, req.MaxAttempts)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if job == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, job)
	}
}

func handleDeleteJob(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		deleted, err := DeleteJob(app.Pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !deleted {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func handleTriggerJob(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		job, err := GetJob(app.Pool, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "job not found"})
				return
			}
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if job == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}

		go app.ExecuteJob(id)

		sdk.Respond(w, http.StatusAccepted, map[string]bool{"triggered": true})
	}
}
