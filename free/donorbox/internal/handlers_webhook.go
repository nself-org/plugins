package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

// --- Webhook -----------------------------------------------------------------

// Size-cap exception: single-responsibility HTTP route handler — 64L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func handleWebhook(db *DB, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
			return
		}

		// Verify HMAC-SHA256 signature if secret is configured
		if secret != "" {
			sig := r.Header.Get("X-Donorbox-Signature")
			if sig == "" {
				sig = r.Header.Get("X-Hub-Signature-256")
			}
			if !verifyHMAC(body, sig, secret) {
				sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
				return
			}
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
			return
		}

		eventType, _ := payload["event_type"].(string)
		if eventType == "" {
			eventType = "donation.created"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Generate unique event ID
		randBytes := make([]byte, 4)
		rand.Read(randBytes)
		eventID := fmt.Sprintf("donorbox_%d_%s", time.Now().UnixMilli(), hex.EncodeToString(randBytes))

		// Store raw event
		if err := db.InsertWebhookEvent(ctx, eventID, eventType, body); err != nil {
			log.Printf("[nself-donorbox] webhook store error: %v", err)
		}

		// Process event
		var processErr *string
		switch eventType {
		case "donation.created":
			if err := processDonationWebhook(ctx, db, payload); err != nil {
				errStr := err.Error()
				processErr = &errStr
				log.Printf("[nself-donorbox] webhook process error: %v", err)
			}
		default:
			log.Printf("[nself-donorbox] unhandled webhook event type: %s", eventType)
		}

		if markErr := db.MarkEventProcessed(ctx, eventID, processErr); markErr != nil {
			log.Printf("[nself-donorbox] mark processed error: %v", markErr)
		}

		sdk.Respond(w, http.StatusOK, map[string]string{"status": "received", "event_id": eventID})
	}
}

