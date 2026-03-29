package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all webhook endpoints on the given router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, dispatcher *Dispatcher) {
	// Endpoint CRUD
	r.Post("/v1/webhooks", handleCreateEndpoint(pool))
	r.Get("/v1/webhooks", handleListEndpoints(pool))
	r.Put("/v1/webhooks/{id}", handleUpdateEndpoint(pool))
	r.Delete("/v1/webhooks/{id}", handleDeleteEndpoint(pool))
	r.Post("/v1/webhooks/{id}/test", handleTestEndpoint(pool, dispatcher))

	// Deliveries
	r.Get("/v1/deliveries", handleListDeliveries(pool))

	// Dispatch
	r.Post("/v1/dispatch", handleDispatch(pool, dispatcher))
}

// --- Request types -----------------------------------------------------------

// CreateEndpointRequest is the JSON body for POST /v1/webhooks.
type CreateEndpointRequest struct {
	URL         string            `json:"url"`
	Events      []string          `json:"events"`
	Secret      string            `json:"secret"`
	Description *string           `json:"description"`
	Headers     map[string]string `json:"headers"`
	Metadata    map[string]string `json:"metadata"`
}

// UpdateEndpointRequest is the JSON body for PUT /v1/webhooks/:id.
type UpdateEndpointRequest struct {
	URL         *string           `json:"url"`
	Events      []string          `json:"events"`
	Description *string           `json:"description"`
	Headers     map[string]string `json:"headers"`
	Enabled     *bool             `json:"enabled"`
	Metadata    map[string]string `json:"metadata"`
}

// DispatchRequest is the JSON body for POST /v1/dispatch.
type DispatchRequest struct {
	EventType      string                 `json:"event_type"`
	Payload        map[string]interface{} `json:"payload"`
	Endpoints      []string               `json:"endpoints"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

// --- Handlers ----------------------------------------------------------------

func handleCreateEndpoint(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateEndpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.URL == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if len(req.Events) == 0 {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "events is required"})
			return
		}

		secret := req.Secret
		if secret == "" {
			var err error
			secret, err = GenerateSecret()
			if err != nil {
				sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate secret"})
				return
			}
		}

		headersJSON := "{}"
		if req.Headers != nil {
			headersJSON = MarshalJSONOrDefault(req.Headers, "{}")
		}
		metadataJSON := "{}"
		if req.Metadata != nil {
			metadataJSON = MarshalJSONOrDefault(req.Metadata, "{}")
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		endpoint, err := CreateEndpoint(ctx, pool, req.URL, req.Events, req.Description, secret, headersJSON, metadataJSON)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to create endpoint: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, endpoint)
	}
}

func handleListEndpoints(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var enabledFilter *bool
		if v := r.URL.Query().Get("enabled"); v != "" {
			b := v == "true"
			enabledFilter = &b
		}

		endpoints, err := ListEndpoints(ctx, pool, enabledFilter)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if endpoints == nil {
			endpoints = []Endpoint{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"endpoints": endpoints,
		})
	}
}

func handleUpdateEndpoint(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		var req UpdateEndpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		var headersJSON *string
		if req.Headers != nil {
			s := MarshalJSONOrDefault(req.Headers, "{}")
			headersJSON = &s
		}
		var metadataJSON *string
		if req.Metadata != nil {
			s := MarshalJSONOrDefault(req.Metadata, "{}")
			metadataJSON = &s
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		endpoint, err := UpdateEndpoint(ctx, pool, id, req.URL, req.Description, req.Events, headersJSON, req.Enabled, metadataJSON)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, endpoint)
	}
}

func handleDeleteEndpoint(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		deleted, err := DeleteEndpoint(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !deleted {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func handleTestEndpoint(pool *pgxpool.Pool, dispatcher *Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		endpoint, err := GetEndpoint(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
			return
		}

		result := dispatcher.TestEndpoint(endpoint)
		if !result.Success {
			sdk.Respond(w, http.StatusBadGateway, result)
			return
		}

		sdk.Respond(w, http.StatusOK, result)
	}
}

func handleListDeliveries(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		endpointID := r.URL.Query().Get("endpoint_id")
		eventType := r.URL.Query().Get("event_type")
		status := r.URL.Query().Get("status")

		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		deliveries, err := ListDeliveries(ctx, pool, endpointID, eventType, status, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if deliveries == nil {
			deliveries = []Delivery{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"deliveries": deliveries,
			"limit":      limit,
			"offset":     offset,
		})
	}
}

func handleDispatch(pool *pgxpool.Pool, dispatcher *Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.EventType == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "event_type is required"})
			return
		}
		if req.Payload == nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "payload is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		result, err := dispatcher.DispatchEvent(ctx, req.EventType, req.Payload, req.Endpoints, req.IdempotencyKey)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusAccepted, result)
	}
}
