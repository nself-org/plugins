// Package server implements the HTTP API for the tenant-controller daemon.
// Routes are protected by the NSELF_CONTROLLER_ADMIN_TOKEN bearer token.
// The server exposes:
//   - POST /projects/create
//   - DELETE /projects/:slug
//   - GET /projects
//   - GET /projects/:slug/status
//   - GET /controller/status
//   - GET /health
//   - GET /metrics  (Prometheus)
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nself-org/nself-tenant-controller/internal/config"
	"github.com/nself-org/nself-tenant-controller/internal/controller"
)

// New constructs the HTTP router for the controller daemon.
func New(cfg *config.Config, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Unauthenticated.
	r.Get("/health", handleHealth)
	r.Handle("/metrics", promhttp.Handler())

	// Admin-token-protected routes.
	r.Group(func(r chi.Router) {
		r.Use(adminAuth(cfg.AdminToken))

		r.Post("/projects/create", handleCreate(cfg, pool))
		r.Delete("/projects/{slug}", handleDelete(cfg, pool))
		r.Get("/projects", handleList(pool))
		r.Get("/projects/{slug}/status", handleProjectStatus(pool))
		r.Get("/controller/status", handleControllerStatus(cfg, pool))
	})

	return r
}

// handleHealth returns 200 OK with a minimal JSON body.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"tenant-controller"}`))
}

// handleCreate provisions a new project.
func handleCreate(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input controller.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		proj, err := controller.Create(r.Context(), pool, cfg, input, controller.InitiatorCLI)
		if err != nil {
			slog.Error("project create failed", "slug", input.Slug, "err", err)
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, proj)
	}
}

// handleDelete tears down a project (requires ?force=true query param).
func handleDelete(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		force := r.URL.Query().Get("force") == "true"

		if err := controller.Delete(r.Context(), pool, cfg, controller.DeleteInput{
			Slug:  slug,
			Force: force,
		}, controller.InitiatorCLI); err != nil {
			slog.Error("project delete failed", "slug", slug, "err", err)
			if strings.Contains(err.Error(), "--force") {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleList lists all projects.
func handleList(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := controller.ListProjects(r.Context(), pool)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, projects)
	}
}

// handleProjectStatus returns per-project health + resource stats.
func handleProjectStatus(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		sr, err := controller.ProjectStatus(r.Context(), pool, slug)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sr)
	}
}

// handleControllerStatus returns status for all projects.
func handleControllerStatus(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, err := controller.AllProjectsStatus(r.Context(), pool, cfg.MaxProjects)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cs)
	}
}

// adminAuth is middleware that validates the X-Controller-Token or Bearer token.
func adminAuth(token string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Controller-Token")
			if got == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					got = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if got != token {
				writeErr(w, http.StatusUnauthorized, "invalid or missing controller token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeJSON serializes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr writes a JSON error response.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
