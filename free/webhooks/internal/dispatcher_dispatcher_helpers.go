package internal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// --- Helpers -----------------------------------------------------------------

func endpointMatchesEvent(ep Endpoint, eventType string) bool {
	for _, evt := range ep.Events {
		if evt == eventType || evt == "*" {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// envIntCapped reads an integer from the environment, falling back to
// defaultVal, and capping at maxVal. This prevents misconfiguration to
// extreme values (e.g. WEBHOOK_DISPATCHER_CONCURRENCY=10000) that would
// re-introduce pool exhaustion.
func envIntCapped(key string, defaultVal, maxVal int) int {
	n := envInt(key, defaultVal)
	if n > maxVal {
		log.Printf("[nself-webhooks] WARN %s=%d exceeds maximum (%d); capping to %d", key, n, maxVal, maxVal)
		return maxVal
	}
	if n <= 0 {
		log.Printf("[nself-webhooks] WARN %s=%d is invalid (must be > 0); using default %d", key, n, defaultVal)
		return defaultVal
	}
	return n
}

func parseRetryDelays(s string) []time.Duration {
	parts := strings.Split(s, ",")
	var delays []time.Duration
	for _, p := range parts {
		p = strings.TrimSpace(p)
		ms, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		delays = append(delays, time.Duration(ms)*time.Millisecond)
	}
	if len(delays) == 0 {
		return []time.Duration{10 * time.Second, 30 * time.Second, 2 * time.Minute, 15 * time.Minute, 1 * time.Hour}
	}
	return delays
}

// emitDLQEvent fires a webhook.delivery.dead_letter internal event (T04).
//
// The event is dispatched back through this plugin's own /v1/dispatch so the
// notify plugin (which subscribes to that event type) can route an alert to
// the operator's admin channel. No PII is included — the payload is hashed.
//
// Opt-out: set NOTIFY_DLQ_ALERTS=false to suppress these events entirely.
// Dedup: the notify plugin is responsible for deduplicating per endpoint_id
// (max one alert per endpoint per hour). See S76-T04 for the notify-side router.
// Size-cap exception: webhook router/dispatcher — 60L event-type dispatch; splitting by event type adds file-per-type overhead without structural gain.
func emitDLQEvent(del Delivery, attemptCount int, lastError *string) {
	if os.Getenv("NOTIFY_DLQ_ALERTS") == "false" {
		return
	}

	selfURL := os.Getenv("WEBHOOKS_INTERNAL_URL")
	if selfURL == "" {
		selfURL = fmt.Sprintf("http://127.0.0.1:%s", os.Getenv("WEBHOOKS_PLUGIN_PORT"))
		if os.Getenv("WEBHOOKS_PLUGIN_PORT") == "" {
			selfURL = "http://127.0.0.1:3403"
		}
	}

	// Hash the payload for the alert — never embed raw payload in an alert.
	mac := hmac.New(sha256.New, []byte("dlq-payload-hash"))
	mac.Write([]byte(del.Payload))
	payloadHash := hex.EncodeToString(mac.Sum(nil))[:16] // first 64 bits is enough for correlation

	errStr := ""
	if lastError != nil {
		errStr = truncate(*lastError, 200)
	}

	body := map[string]interface{}{
		"event_type":    "webhook.delivery.dead_letter",
		"endpoint_id":   del.EndpointID,
		"delivery_id":   del.ID,
		"event_type_dl": del.EventType,
		"attempt_count": attemptCount,
		"last_error":    errStr,
		"payload_hash":  payloadHash,
	}
	data, err := json.Marshal(map[string]interface{}{
		"event_type": "webhook.delivery.dead_letter",
		"payload":    body,
	})
	if err != nil {
		log.Printf("[nself-webhooks] DLQ event marshal error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, selfURL+"/v1/dispatch", bytes.NewReader(data))
	if err != nil {
		log.Printf("[nself-webhooks] DLQ event request build error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[nself-webhooks] DLQ event dispatch error: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("[nself-webhooks] DLQ event dispatched for delivery %s, status=%d", del.ID, resp.StatusCode)
}

