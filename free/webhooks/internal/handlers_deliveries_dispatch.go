package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

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
		// Payload size cap (T03): reject oversized bodies before decoding.
		// Uses http.MaxBytesReader so the TCP connection is not left in a bad state.
		cap := maxPayloadBytes()
		r.Body = http.MaxBytesReader(w, r.Body, int64(cap))

		var req DispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// MaxBytesReader wraps the error when the limit is exceeded.
			if err.Error() == "http: request body too large" {
				sdk.Respond(w, http.StatusRequestEntityTooLarge, map[string]string{
					"error": "payload exceeds WEBHOOKS_MAX_PAYLOAD_BYTES",
				})
				return
			}
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
