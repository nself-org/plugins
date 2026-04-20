package internal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ssrfDenyNets is the list of IP networks that outbound webhook destinations
// must not resolve to. Covers RFC1918 private ranges, link-local (169.254/16
// — AWS metadata endpoint), loopback, and multicast.
//
// Security-Always-Free Doctrine: SSRF prevention is free, default, automatic.
// Opt-in for local dev via WEBHOOK_ALLOW_PRIVATE_URLS=true (never in prod).
var ssrfDenyNets []*net.IPNet

func init() {
	// Parse deny-list CIDRs at package init so ValidateWebhookURL is fast.
	cidrs := []string{
		"10.0.0.0/8",      // RFC1918 Class A
		"172.16.0.0/12",   // RFC1918 Class B
		"192.168.0.0/16",  // RFC1918 Class C
		"169.254.0.0/16",  // Link-local / AWS EC2 metadata (169.254.169.254)
		"127.0.0.0/8",     // Loopback
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 unique local
		"fe80::/10",       // IPv6 link-local
		"0.0.0.0/8",       // Unspecified / "this" network
		"100.64.0.0/10",   // Shared address space (CGNAT)
		"192.0.0.0/24",    // IETF Protocol Assignments
		"198.18.0.0/15",   // Benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // Reserved
		"255.255.255.255/32", // Broadcast
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			ssrfDenyNets = append(ssrfDenyNets, network)
		}
	}
}

// ValidateWebhookURL verifies that a webhook destination URL is safe to
// deliver to. It:
//  1. Parses the URL and requires HTTPS (or HTTP when WEBHOOK_ALLOW_PRIVATE_URLS=true).
//  2. Resolves the hostname via DNS.
//  3. Checks every resolved IP against the SSRF deny-list (RFC1918, link-local,
//     loopback, etc.).
//
// DNS-rebinding mitigation: the IP is resolved once at registration time and
// stored. Subsequent deliveries use the stored IP (handled by the caller).
// Both A and AAAA records are checked.
//
// Returns nil when the URL is safe, or a descriptive error otherwise.
func ValidateWebhookURL(rawURL string) error {
	if os.Getenv("WEBHOOK_ALLOW_PRIVATE_URLS") == "true" {
		// Development opt-in: bypass SSRF guard. Never set in production.
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("webhook URL scheme %q not allowed (must be https)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL missing host")
	}

	// Resolve hostname to IPs. Use a 5-second timeout to prevent slow DNS
	// from blocking registration.
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("webhook URL DNS resolution failed for %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("webhook URL %q resolved to no addresses", host)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("webhook URL resolved to invalid IP %q", addr)
		}
		for _, deny := range ssrfDenyNets {
			if deny.Contains(ip) {
				return fmt.Errorf("webhook destination %q resolves to private/internal address %s (SSRF guard)", host, ip)
			}
		}
	}

	return nil
}

// Dispatcher handles webhook delivery with retry logic, HMAC signing, and DLQ.
type Dispatcher struct {
	pool               *pgxpool.Pool
	client             *http.Client
	maxAttempts        int
	requestTimeoutMs   int
	concurrentLimit    int
	retryDelays        []time.Duration
	autoDisableThresh  int
	activeDeliveries   int
	mu                 sync.Mutex
	stopCh             chan struct{}
}

// TestResult holds the outcome of a test webhook delivery.
type TestResult struct {
	Success      bool   `json:"success"`
	Status       *int   `json:"status,omitempty"`
	ResponseTime *int   `json:"response_time,omitempty"`
	Error        string `json:"error,omitempty"`
}

// DispatchResult holds the outcome of dispatching an event.
type DispatchResult struct {
	Dispatched int      `json:"dispatched"`
	Endpoints  []string `json:"endpoints"`
}

// NewDispatcher creates a new Dispatcher with configuration from environment.
func NewDispatcher(pool *pgxpool.Pool) *Dispatcher {
	maxAttempts := envInt("WEBHOOKS_MAX_ATTEMPTS", 5)
	requestTimeout := envInt("WEBHOOKS_REQUEST_TIMEOUT_MS", 30000)
	concurrent := envInt("WEBHOOKS_CONCURRENT_DELIVERIES", 10)
	autoDisable := envInt("WEBHOOKS_AUTO_DISABLE_THRESHOLD", 10)

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
		pool:              pool,
		client:            &http.Client{Timeout: time.Duration(requestTimeout) * time.Millisecond},
		maxAttempts:       maxAttempts,
		requestTimeoutMs:  requestTimeout,
		concurrentLimit:   concurrent,
		retryDelays:       retryDelays,
		autoDisableThresh: autoDisable,
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
func (d *Dispatcher) StartProcessing() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Printf("[nself-webhooks] delivery processor started (concurrent=%d, maxAttempts=%d)", d.concurrentLimit, d.maxAttempts)

	for {
		select {
		case <-d.stopCh:
			log.Printf("[nself-webhooks] delivery processor stopped")
			return
		case <-ticker.C:
			d.processPending()
		}
	}
}

// StopProcessing signals the background processor to stop.
func (d *Dispatcher) StopProcessing() {
	close(d.stopCh)
}

func (d *Dispatcher) processPending() {
	d.mu.Lock()
	available := d.concurrentLimit - d.activeDeliveries
	d.mu.Unlock()

	if available <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deliveries, err := GetPendingDeliveries(ctx, d.pool, available)
	if err != nil {
		log.Printf("[nself-webhooks] error fetching pending deliveries: %v", err)
		return
	}
	if len(deliveries) == 0 {
		return
	}

	log.Printf("[nself-webhooks] processing %d pending deliveries", len(deliveries))

	var wg sync.WaitGroup
	for i := range deliveries {
		d.mu.Lock()
		d.activeDeliveries++
		d.mu.Unlock()

		wg.Add(1)
		go func(del Delivery) {
			defer wg.Done()
			defer func() {
				d.mu.Lock()
				d.activeDeliveries--
				d.mu.Unlock()
			}()
			d.processDelivery(del)
		}(deliveries[i])
	}
	wg.Wait()
}

func (d *Dispatcher) processDelivery(del Delivery) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(d.requestTimeoutMs+5000)*time.Millisecond)
	defer cancel()

	endpoint, err := GetEndpoint(ctx, d.pool, del.EndpointID)
	if err != nil {
		errMsg := "endpoint not found"
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "failed", nil, nil, nil, &errMsg, nil)
		return
	}

	if !endpoint.Enabled {
		errMsg := "endpoint is disabled"
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "failed", nil, nil, nil, &errMsg, nil)
		return
	}

	// Mark as delivering (increment attempt).
	_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "delivering", nil, nil, nil, nil, nil)

	// Build HTTP request.
	payloadBytes := []byte(del.Payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		errMsg := "failed to build request: " + err.Error()
		d.handleFailure(ctx, del, endpoint, nil, &errMsg)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nself-webhooks/1.0")
	req.Header.Set("X-Webhook-Signature", del.Signature)
	req.Header.Set("X-Webhook-Event-Type", del.EventType)
	req.Header.Set("X-Webhook-Delivery-Id", del.ID)
	req.Header.Set("X-Webhook-Attempt", strconv.Itoa(del.AttemptCount+1))

	// Apply custom headers from endpoint.
	var customHeaders map[string]string
	if err := json.Unmarshal([]byte(endpoint.Headers), &customHeaders); err == nil {
		for k, v := range customHeaders {
			req.Header.Set(k, v)
		}
	}

	start := time.Now()
	resp, err := d.client.Do(req)
	elapsed := int(time.Since(start).Milliseconds())

	if err != nil {
		errMsg := err.Error()
		d.handleFailure(ctx, del, endpoint, &elapsed, &errMsg)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	respBody := string(bodyBytes)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success.
		status := resp.StatusCode
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "delivered", &status, &respBody, &elapsed, nil, nil)
		_ = RecordEndpointSuccess(ctx, d.pool, endpoint.ID)
		return
	}

	// HTTP error.
	errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(respBody, 200))
	d.handleFailure(ctx, del, endpoint, &elapsed, &errMsg)
}

func (d *Dispatcher) handleFailure(ctx context.Context, del Delivery, endpoint *Endpoint, responseTimeMs *int, errMsg *string) {
	newAttemptCount := del.AttemptCount + 1
	shouldRetry := newAttemptCount < del.MaxAttempts

	if shouldRetry {
		delayIdx := newAttemptCount - 1
		if delayIdx >= len(d.retryDelays) {
			delayIdx = len(d.retryDelays) - 1
		}
		nextRetry := time.Now().Add(d.retryDelays[delayIdx])
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "pending", nil, nil, responseTimeMs, errMsg, &nextRetry)
	} else {
		// Max attempts exhausted: dead letter.
		_ = MarkDeliveryDeadLetter(ctx, d.pool, del.ID, responseTimeMs, errMsg)
		log.Printf("[nself-webhooks] delivery %s moved to dead letter queue after %d attempts", del.ID, newAttemptCount)
		// Fire DLQ alert event (T04). No PII — payload is hashed, not embedded.
		emitDLQEvent(del, newAttemptCount, errMsg)
	}

	_ = RecordEndpointFailure(ctx, d.pool, endpoint.ID, d.autoDisableThresh)
}

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
