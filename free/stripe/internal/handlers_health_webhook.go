package internal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

// ============================================================================
// Health Checks
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"plugin":    "stripe",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	err := s.DB.Pool.Ping(r.Context())
	if err != nil {
		log.Printf("[stripe:health] Readiness check failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ready":     false,
			"plugin":    "stripe",
			"error":     "Database unavailable",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":     true,
		"plugin":    "stripe",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	stats, err := db.GetStats(r.Context())
	if err != nil {
		log.Printf("[stripe:health] Live check stats failed: %v", err)
		stats = &SyncStats{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alive":   true,
		"plugin":  "stripe",
		"version": "1.0.0",
		"uptime":  time.Since(s.StartAt).Seconds(),
		"stats": map[string]interface{}{
			"customers":     stats.Customers,
			"subscriptions": stats.Subscriptions,
			"lastSync":      stats.LastSyncedAt,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	stats, err := db.GetStats(r.Context())
	if err != nil {
		log.Printf("[stripe:health] Status stats failed: %v", err)
		stats = &SyncStats{}
	}

	accountIDs := make([]string, len(s.Accounts))
	for i, acc := range s.Accounts {
		accountIDs[i] = acc.ID
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"plugin":    "stripe",
		"version":   "1.0.0",
		"status":    "running",
		"accounts":  accountIDs,
		"stats":     stats,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// ============================================================================
// Webhook
// ============================================================================

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("Stripe-Signature")
	if signature == "" {
		log.Println("[stripe:webhooks] Missing Stripe signature header")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing signature"})
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[stripe:webhooks] Failed to read body: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read body"})
		return
	}
	defer r.Body.Close()

	// Find matching account by signature
	matchIdx := FindMatchingAccount(rawBody, signature, s.Accounts)
	if matchIdx < 0 {
		log.Println("[stripe:webhooks] Invalid Stripe signature for all configured accounts")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid signature"})
		return
	}

	matchedAccount := s.Accounts[matchIdx]

	if matchedAccount.WebhookSecret == "" {
		log.Println("[stripe:webhooks] Webhook secret not configured for matched account")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Webhook secret not configured"})
		return
	}

	// Parse the event
	var event StripeEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		log.Printf("[stripe:webhooks] Failed to parse event: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid event payload"})
		return
	}

	// Process the event with the matched account's scoped DB
	scopedDB := s.DB.ForSourceAccount(matchedAccount.ID)

	// Idempotency check (S76-T05): have we already processed this Stripe event ID?
	// Mirrors the pattern in ping_api/src/routes/webhooks.ts (line 156) so both
	// canonical paths guarantee at-most-once processing. The stripe_events table
	// is created by the plugin migration (schema/tables.sql) with IF NOT EXISTS.
	if event.ID != "" {
		var exists bool
		_ = scopedDB.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM stripe_events WHERE stripe_event_id = $1 AND source_account_id = $2)`,
			event.ID, scopedDB.SourceAccountID,
		).Scan(&exists)
		if exists {
			log.Printf("[stripe:webhooks] Event already processed (idempotent): %s", event.ID)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"received":   true,
				"idempotent": true,
			})
			return
		}
	}

	handler := NewWebhookHandler(scopedDB)

	if err := handler.HandleEvent(r.Context(), &event); err != nil {
		log.Printf("[stripe:webhooks] Processing failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Processing failed"})
		return
	}

	// Record processed event for idempotency. Non-fatal if stripe_events table
	// doesn't exist yet (e.g., plugin just installed, migration pending).
	if event.ID != "" {
		_, err := scopedDB.Pool.Exec(r.Context(),
			`INSERT INTO stripe_events (stripe_event_id, source_account_id, event_type, processed_at)
			 VALUES ($1, $2, $3, NOW())
			 ON CONFLICT (stripe_event_id) DO NOTHING`,
			event.ID, scopedDB.SourceAccountID, event.Type,
		)
		if err != nil {
			log.Printf("[stripe:webhooks] Warning: could not record event %s for idempotency: %v", event.ID, err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"received": true,
		"account":  matchedAccount.ID,
	})
}
