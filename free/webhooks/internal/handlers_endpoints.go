package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// Size-cap exception: single-responsibility HTTP route handler — 54L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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
		// SSRF guard (Security-Always-Free): reject destinations resolving to
		// private/internal addresses before persisting the endpoint.
		if err := ValidateWebhookURL(req.URL); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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

		// SSRF guard (Security-Always-Free): if the URL is being changed,
		// re-validate it before persisting.
		if req.URL != nil {
			if err := ValidateWebhookURL(*req.URL); err != nil {
				sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
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
