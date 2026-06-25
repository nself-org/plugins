package internal

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleWebhook processes incoming PayPal webhook events.
// It validates the request structure, stores the raw event, and routes
// to the appropriate handler based on event_type.
// Size-cap exception: single-responsibility HTTP route handler — 71L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func HandleWebhook(pool *pgxpool.Pool, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Read the raw body.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Basic structural validation: verify the Transmission-Id header exists
		// and the body parses as valid JSON with required fields.
		transmissionID := r.Header.Get("Paypal-Transmission-Id")
		if transmissionID == "" {
			log.Printf("[nself-paypal] webhook: missing Paypal-Transmission-Id header")
			http.Error(w, "missing transmission id", http.StatusBadRequest)
			return
		}

		var event webhookPayload
		if err := json.Unmarshal(body, &event); err != nil {
			log.Printf("[nself-paypal] webhook: invalid JSON: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if event.ID == "" || event.EventType == "" {
			log.Printf("[nself-paypal] webhook: missing id or event_type")
			http.Error(w, "missing required fields", http.StatusBadRequest)
			return
		}

		// Store the raw webhook event.
		summary := nilIfEmpty(event.Summary)
		createTime := parseTimePtr(event.CreateTime)

		resource := event.Resource
		if len(resource) == 0 {
			resource = json.RawMessage("{}")
		}

		err = InsertWebhookEvent(ctx, pool, &WebhookEvent{
			PayPalEventID:   event.ID,
			EventType:       event.EventType,
			ResourceType:    event.ResourceType,
			Resource:        resource,
			Summary:         summary,
			CreateTime:      createTime,
			Processed:       false,
			SourceAccountID: "primary",
		})
		if err != nil {
			log.Printf("[nself-paypal] webhook: failed to store event %s: %v", event.ID, err)
		}

		// Route to the appropriate handler.
		processErr := routeEvent(ctx, pool, &event)
		if processErr != nil {
			log.Printf("[nself-paypal] webhook: error processing %s (%s): %v", event.EventType, event.ID, processErr)
			// Still return 200 to prevent PayPal from retrying on processing errors.
		} else {
			log.Printf("[nself-paypal] webhook: processed %s (%s)", event.EventType, event.ID)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`))
	}
}

// webhookPayload represents the incoming PayPal webhook event structure.
type webhookPayload struct {
	ID           string          `json:"id"`
	EventType    string          `json:"event_type"`
	ResourceType string          `json:"resource_type"`
	Resource     json.RawMessage `json:"resource"`
	Summary      string          `json:"summary"`
	CreateTime   string          `json:"create_time"`
}

// routeEvent dispatches webhook events to the correct handler based on event_type prefix.
