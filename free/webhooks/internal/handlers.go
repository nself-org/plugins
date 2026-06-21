package internal

import (

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
