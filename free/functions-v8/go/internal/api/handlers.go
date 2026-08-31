// Package api provides HTTP handlers for the functions-edge plugin.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/nself-org/nself-functions-v8/internal/deploy"
	"github.com/nself-org/nself-functions-v8/internal/env"
	"github.com/nself-org/nself-functions-v8/internal/isolate"
	"github.com/nself-org/nself-functions-v8/internal/invoke"
	"github.com/nself-org/nself-functions-v8/internal/logs"
	"github.com/nself-org/nself-functions-v8/internal/store"
)

// Handler groups all HTTP handlers for the plugin.
type Handler struct {
	store       *store.Store
	pool        *isolate.Pool
	logger      *logs.Logger
	log         *slog.Logger
	typeChecker *deploy.TypeChecker
}

// New creates a new Handler.
func New(st *store.Store, pool *isolate.Pool, lgr *logs.Logger, log *slog.Logger, checker *deploy.TypeChecker) *Handler {
	return &Handler{store: st, pool: pool, logger: lgr, log: log, typeChecker: checker}
}

// jsonOK writes a JSON 200 response.
func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// jsonErr writes a JSON error response.
func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

// Deploy handles POST /api/functions/edge — type-check then deploy a new function.
func (h *Handler) Deploy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		SourceCode  string `json:"source_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	// Type-check the TypeScript source before persisting.
	if h.typeChecker != nil {
		if err := h.typeChecker.Check(r.Context(), req.SourceCode); err != nil {
			var tcErr *deploy.TypeCheckError
			if errors.As(err, &tcErr) {
				jsonErr(w, http.StatusUnprocessableEntity, tcErr.Message)
				return
			}
			// Non-type-check failure (e.g. temp file error) — log and continue.
			h.log.Warn("type-check skipped due to internal error", "err", err)
		}
	}

	version, err := h.store.CreateOrUpdate(r.Context(), req.Name, req.Description, req.SourceCode, "api")
	if errors.Is(err, store.ErrInvalidFunctionName) {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, store.ErrSourceTooLarge) {
		jsonErr(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if err != nil {
		h.log.Error("deploy failed", "err", err)
		jsonErr(w, http.StatusInternalServerError, "deploy failed")
		return
	}
	jsonOK(w, map[string]any{
		"name":       req.Name,
		"version":    version,
		"invoke_url": "/functions/v1/" + req.Name,
	})
}

// List handles GET /api/functions/edge.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	fns, err := h.store.List(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"functions": fns})
}

// Get handles GET /api/functions/edge/:name.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fn, err := h.store.GetByName(r.Context(), name)
	if errors.Is(err, store.ErrFunctionNotFound) {
		jsonErr(w, http.StatusNotFound, "function not found")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	vers, _ := h.store.ListVersions(r.Context(), name)
	jsonOK(w, map[string]any{"function": fn, "versions": vers})
}

// Delete handles DELETE /api/functions/edge/:name.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.store.Delete(r.Context(), name); errors.Is(err, store.ErrFunctionNotFound) {
		jsonErr(w, http.StatusNotFound, "function not found")
		return
	} else if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Activate handles PATCH /api/functions/edge/:name/activate.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.ActivateVersion(r.Context(), name, req.Version); err != nil {
		if errors.Is(err, store.ErrFunctionNotFound) {
			jsonErr(w, http.StatusNotFound, err.Error())
		} else {
			jsonErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	jsonOK(w, map[string]any{"name": name, "active_version": req.Version})
}

// Invoke handles GET/POST /functions/v1/:name — invoke a function.
func (h *Handler) Invoke(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// Acquire a pool slot.
	if err := h.pool.Acquire(r.Context()); err != nil {
		jsonErr(w, http.StatusServiceUnavailable, "no isolate slots available")
		return
	}
	defer h.pool.Release()

	// Load function source + env.
	fnID, source, err := h.store.GetActive(r.Context(), name)
	if errors.Is(err, store.ErrFunctionNotFound) {
		jsonErr(w, http.StatusNotFound, "function not found or paused")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "failed to load function")
		return
	}

	envVars, _ := h.store.GetEnvVars(r.Context(), name)
	envMap := make(map[string]string, len(envVars))
	for _, e := range envVars {
		envMap[e.Key] = e.Value
	}
	// Build clean env — all declared keys are already the allowlist.
	allowedKeys := make([]string, 0, len(envMap))
	for k := range envMap {
		allowedKeys = append(allowedKeys, k)
	}
	safeEnv := env.BuildEnv(envMap, allowedKeys)

	body, _ := io.ReadAll(r.Body)
	result, invokeErr := h.pool.Invoke(r.Context(), source, safeEnv, r.Method, r.URL.Path, body)
	isError := invokeErr != nil || (result != nil && result.StatusCode >= 500)
	go h.store.IncrementInvokeCount(context.Background(), fnID, isError)

	if invokeErr != nil {
		invoke.RecordInvocation(name, 500, 0, false)
		jsonErr(w, http.StatusInternalServerError, "invocation failed")
		return
	}

	// Record Prometheus metrics.
	invoke.RecordInvocation(name, result.StatusCode, result.Duration, result.TimedOut)

	// Persist log lines asynchronously.
	if len(result.LogLines) > 0 {
		level := "info"
		if isError {
			level = "error"
		}
		go h.logger.Write(context.Background(), fnID, level, result.LogLines)
	}

	if result.TimedOut {
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write(result.Body) //nolint:errcheck
		return
	}

	for k, v := range result.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(result.StatusCode)
	w.Write(result.Body) //nolint:errcheck
}

// ListLogs handles GET /api/functions/edge/:name/logs.
func (h *Handler) ListLogs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fn, err := h.store.GetByName(r.Context(), name)
	if errors.Is(err, store.ErrFunctionNotFound) {
		jsonErr(w, http.StatusNotFound, "function not found")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := h.logger.ListRecent(r.Context(), fn.ID, limit)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"logs": entries})
}

// StreamLogs handles GET /api/functions/edge/:name/logs/stream (SSE).
func (h *Handler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fn, err := h.store.GetByName(r.Context(), name)
	if errors.Is(err, store.ErrFunctionNotFound) {
		jsonErr(w, http.StatusNotFound, "function not found")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.StreamSSE(r.Context(), w, fn.ID)
}

// GetEnv handles GET /api/functions/edge/:name/env.
func (h *Handler) GetEnv(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	vars, err := h.store.GetEnvVars(r.Context(), name)
	if errors.Is(err, store.ErrFunctionNotFound) {
		jsonErr(w, http.StatusNotFound, "function not found")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return keys only — values are sensitive.
	keys := make([]string, 0, len(vars))
	for _, v := range vars {
		keys = append(keys, v.Key)
	}
	jsonOK(w, map[string]any{"env_keys": keys})
}

// SetEnv handles POST /api/functions/edge/:name/env.
func (h *Handler) SetEnv(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Key == "" {
		jsonErr(w, http.StatusBadRequest, "key is required")
		return
	}
	if err := h.store.SetEnvVar(r.Context(), name, req.Key, req.Value); err != nil {
		if errors.Is(err, store.ErrFunctionNotFound) {
			jsonErr(w, http.StatusNotFound, "function not found")
		} else {
			jsonErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	jsonOK(w, map[string]any{"key": req.Key, "set": true})
}

// DeleteEnv handles DELETE /api/functions/edge/:name/env/:key.
func (h *Handler) DeleteEnv(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	key := chi.URLParam(r, "key")
	if err := h.store.DeleteEnvVar(r.Context(), name, key); err != nil {
		if errors.Is(err, store.ErrFunctionNotFound) {
			jsonErr(w, http.StatusNotFound, "function not found")
		} else {
			jsonErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Health handles GET /health — also reports pool state for nself doctor.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ps := h.pool.PoolSize()
	invoke.RecordPoolState(ps, ps) // best-effort gauge; real available count needs semaphore introspection
	jsonOK(w, map[string]any{
		"status":    "ok",
		"pool_size": ps,
	})
}

// RegisterRoutes wires all handlers to a chi router.
func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/health", h.Health)
	r.Handle("/metrics", invoke.Handler())

	// Management API
	r.Route("/api/functions/edge", func(r chi.Router) {
		r.Post("/", h.Deploy)
		r.Get("/", h.List)
		r.Get("/{name}", h.Get)
		r.Delete("/{name}", h.Delete)
		r.Patch("/{name}/activate", h.Activate)
		r.Get("/{name}/logs", h.ListLogs)
		r.Get("/{name}/logs/stream", h.StreamLogs)
		r.Get("/{name}/env", h.GetEnv)
		r.Post("/{name}/env", h.SetEnv)
		r.Delete("/{name}/env/{key}", h.DeleteEnv)
	})

	// Public invocation path (proxied via Nginx from /functions/v1/)
	r.Get("/functions/v1/{name}", h.Invoke)
	r.Post("/functions/v1/{name}", h.Invoke)
}

// Ensure uuid is used (it is via store/logs types surfaced in handlers).
var _ = uuid.Nil
