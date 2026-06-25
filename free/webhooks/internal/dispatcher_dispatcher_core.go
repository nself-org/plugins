package internal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"os"
	"time"
)

func NewDispatcher(pool *pgxpool.Pool) *Dispatcher {
	maxAttempts := envInt("WEBHOOKS_MAX_ATTEMPTS", 5)
	requestTimeout := envInt("WEBHOOKS_REQUEST_TIMEOUT_MS", 30000)
	autoDisable := envInt("WEBHOOKS_AUTO_DISABLE_THRESHOLD", 10)
	maxConcurrency := envIntCapped("WEBHOOK_DISPATCHER_CONCURRENCY", 50, 200)

	retryDelays := []time.Duration{
		10 * time.Second,
		30 * time.Second,
		2 * time.Minute,
		15 * time.Minute,
		1 * time.Hour,
	}
	if v := os.Getenv("WEBHOOKS_RETRY_DELAYS"); v != "" {
		retryDelays = parseRetryDelays(v)
	}

	return &Dispatcher{
		pool: pool,
		client: &http.Client{
			Timeout: time.Duration(requestTimeout) * time.Millisecond,
			// SSRF guard: re-validate every redirect target so a public host
			// cannot redirect to a private/internal address (Security-Always-Free).
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("SSRF guard: too many redirects (max 3)")
				}
				if err := ValidateWebhookURL(req.URL.String()); err != nil {
					return fmt.Errorf("SSRF guard: redirect target blocked: %w", err)
				}
				return nil
			},
		},
		maxAttempts:       maxAttempts,
		requestTimeoutMs:  requestTimeout,
		retryDelays:       retryDelays,
		autoDisableThresh: autoDisable,
		sem:               make(chan struct{}, maxConcurrency),
		maxConcurrency:    maxConcurrency,
		stopCh:            make(chan struct{}),
	}
}

// GenerateSignature creates an HMAC-SHA256 signature for a webhook payload.
func GenerateSignature(payload []byte, secret string) string {
	timestamp := time.Now().Unix()
	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, sig)
}

// maxPayloadBytes returns the configured payload size cap (default 1 MB).
func maxPayloadBytes() int {
	return envInt("WEBHOOKS_MAX_PAYLOAD_BYTES", 1048576)
}

// DispatchEvent finds matching endpoints and creates delivery records.
// Size-cap exception: webhook router/dispatcher — 77L event-type dispatch; splitting by event type adds file-per-type overhead without structural gain.
func (d *Dispatcher) DispatchEvent(ctx context.Context, eventType string, payload map[string]interface{}, targetEndpointIDs []string, idempotencyKey string) (*DispatchResult, error) {
	enabledTrue := true
	allEndpoints, err := ListEndpoints(ctx, d.pool, &enabledTrue)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}

	// Filter by event subscription.
	var matching []Endpoint
	for _, ep := range allEndpoints {
		if endpointMatchesEvent(ep, eventType) {
			matching = append(matching, ep)
		}
	}

	// Filter by specific endpoint IDs if provided.
	if len(targetEndpointIDs) > 0 {
		idSet := make(map[string]bool, len(targetEndpointIDs))
		for _, id := range targetEndpointIDs {
			idSet[id] = true
		}
		var filtered []Endpoint
		for _, ep := range matching {
			if idSet[ep.ID] {
				filtered = append(filtered, ep)
			}
		}
		matching = filtered
	}

	if len(matching) == 0 {
		return &DispatchResult{Dispatched: 0, Endpoints: []string{}}, nil
	}

	// Build full payload with metadata.
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("%s_%d", eventType, time.Now().UnixMilli())
	}
	fullPayload := make(map[string]interface{})
	for k, v := range payload {
		fullPayload[k] = v
	}
	fullPayload["event_type"] = eventType
	fullPayload["idempotency_key"] = idempotencyKey
	fullPayload["timestamp"] = time.Now().UTC().Format(time.RFC3339)

	payloadBytes, err := json.Marshal(fullPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	// Payload size cap (T03): defense-in-depth check at dispatch time.
	// The HTTP handler also checks at request-read time (handlers.go).
	cap := maxPayloadBytes()
	if len(payloadBytes) > cap {
		log.Printf("[nself-webhooks] dispatch rejected: payload size %d exceeds WEBHOOKS_MAX_PAYLOAD_BYTES=%d for event_type=%s", len(payloadBytes), cap, eventType)
		return nil, fmt.Errorf("payload size %d exceeds WEBHOOKS_MAX_PAYLOAD_BYTES=%d", len(payloadBytes), cap)
	}

	payloadJSON := string(payloadBytes)

	var endpointIDs []string
	for _, ep := range matching {
		sig := GenerateSignature(payloadBytes, ep.Secret)
		_, err := CreateDelivery(ctx, d.pool, ep.ID, eventType, payloadJSON, sig, d.maxAttempts)
		if err != nil {
			log.Printf("[nself-webhooks] failed to create delivery for endpoint %s: %v", ep.ID, err)
			continue
		}
		endpointIDs = append(endpointIDs, ep.ID)
	}

	return &DispatchResult{
		Dispatched: len(endpointIDs),
		Endpoints:  endpointIDs,
	}, nil
}

// TestEndpoint sends a test event to a specific endpoint synchronously.
func (d *Dispatcher) TestEndpoint(endpoint *Endpoint) TestResult {
	testPayload := map[string]interface{}{
		"event_type": "test.webhook",
		"test":       true,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"message":    "This is a test webhook from nself",
	}

	// SSRF guard (second layer, DNS-rebinding defense): re-validate the
	// destination at delivery time, not just at registration.
	if err := ValidateWebhookURL(endpoint.URL); err != nil {
		return TestResult{Success: false, Error: err.Error()}
	}

	payloadBytes, _ := json.Marshal(testPayload)
	sig := GenerateSignature(payloadBytes, endpoint.Secret)

	req, err := http.NewRequest(http.MethodPost, endpoint.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return TestResult{Success: false, Error: "failed to build request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nself-webhooks/1.0")
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("X-Webhook-Event-Type", "test.webhook")
	req.Header.Set("X-Webhook-Test", "true")

	start := time.Now()
	resp, err := d.client.Do(req)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return TestResult{Success: false, ResponseTime: &elapsed, Error: err.Error()}
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	success := status >= 200 && status < 300
	result := TestResult{
		Success:      success,
		Status:       &status,
		ResponseTime: &elapsed,
	}
	if !success {
		result.Error = fmt.Sprintf("HTTP %d", status)
	}
	return result
}

// StartProcessing begins background polling for pending deliveries.
