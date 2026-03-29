package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

// DefaultTimestampTolerance is the maximum age of a webhook event signature (5 minutes).
const DefaultTimestampTolerance = 300 * time.Second

// StripeEvent represents a parsed Stripe webhook event payload.
type StripeEvent struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	APIVersion      string          `json:"api_version"`
	Created         int64           `json:"created"`
	Livemode        bool            `json:"livemode"`
	PendingWebhooks int32           `json:"pending_webhooks"`
	Request         *EventRequest   `json:"request"`
	Data            EventData       `json:"data"`
}

// EventRequest contains the request metadata from a Stripe event.
type EventRequest struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
}

// EventData contains the object payload from a Stripe event.
type EventData struct {
	Object json.RawMessage `json:"object"`
}

// VerifyStripeSignature verifies a Stripe webhook signature.
// The signature header has the format: t=timestamp,v1=signature[,v1=signature...]
// The expected signature is HMAC-SHA256(timestamp + "." + rawBody, webhookSecret).
// Returns nil if the signature is valid, or an error describing the failure.
func VerifyStripeSignature(rawBody []byte, signatureHeader string, webhookSecret string) error {
	if signatureHeader == "" {
		return fmt.Errorf("missing signature header")
	}
	if webhookSecret == "" {
		return fmt.Errorf("webhook secret not configured")
	}

	// Parse the signature header
	parts := strings.Split(signatureHeader, ",")
	var timestamp string
	var signatures []string

	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" {
		return fmt.Errorf("missing timestamp in signature header")
	}
	if len(signatures) == 0 {
		return fmt.Errorf("missing v1 signature in signature header")
	}

	// Verify timestamp is within tolerance
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp in signature header: %w", err)
	}

	diff := time.Since(time.Unix(ts, 0))
	if diff < 0 {
		diff = -diff
	}
	if diff > DefaultTimestampTolerance {
		return fmt.Errorf("timestamp outside tolerance: %v old (max %v)", diff, DefaultTimestampTolerance)
	}

	// Compute expected signature: HMAC-SHA256(timestamp + "." + rawBody, secret)
	signedPayload := timestamp + "." + string(rawBody)
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(signedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	// Compare with constant-time comparison against each v1 signature
	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expectedSig)) {
			return nil
		}
	}

	return fmt.Errorf("no matching v1 signature found")
}

// WebhookHandler processes incoming Stripe webhook events.
type WebhookHandler struct {
	db *DB
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(db *DB) *WebhookHandler {
	return &WebhookHandler{db: db}
}

// HandleEvent processes a verified Stripe webhook event:
// 1. Stores the event in np_stripe_webhook_events
// 2. Dispatches to the appropriate handler based on event type
// 3. Marks the event as processed
func (wh *WebhookHandler) HandleEvent(ctx ContextKey, event *StripeEvent) error {
	// Determine object type and id from the event data
	objectType, objectID := extractObjectInfo(event.Data.Object)

	// Build the webhook event record
	record := &StripeWebhookEvent{
		ID:         event.ID,
		Type:       event.Type,
		APIVersion: NullString{String: event.APIVersion, Valid: event.APIVersion != ""},
		Data:       event.Data.Object,
		ObjectType: NullString{String: objectType, Valid: objectType != ""},
		ObjectID:   NullString{String: objectID, Valid: objectID != ""},
		Livemode:   event.Livemode,
		PendingWebhooks: event.PendingWebhooks,
		Processed:  false,
		RetryCount: 0,
		CreatedAt:  NullTime{Time: time.Unix(event.Created, 0), Valid: true},
		ReceivedAt: NullTime{Time: time.Now(), Valid: true},
	}

	if event.Request != nil {
		record.RequestID = NullString{String: event.Request.ID, Valid: event.Request.ID != ""}
		record.RequestIdempotencyKey = NullString{String: event.Request.IdempotencyKey, Valid: event.Request.IdempotencyKey != ""}
	}

	// Store the event
	if err := wh.db.InsertWebhookEvent(ctx, record); err != nil {
		log.Printf("[stripe:webhooks] Failed to store event %s: %v", event.ID, err)
		return err
	}

	log.Printf("[stripe:webhooks] Event received: type=%s id=%s", event.Type, event.ID)

	// Dispatch based on event type
	err := wh.dispatch(ctx, event, objectType, objectID)
	if err != nil {
		log.Printf("[stripe:webhooks] Event processing failed: type=%s id=%s error=%v", event.Type, event.ID, err)
		_ = wh.db.MarkEventProcessed(ctx, event.ID, err.Error())
		return err
	}

	// Mark as processed
	if markErr := wh.db.MarkEventProcessed(ctx, event.ID, ""); markErr != nil {
		log.Printf("[stripe:webhooks] Failed to mark event processed: %v", markErr)
	}

	log.Printf("[stripe:webhooks] Event processed: type=%s id=%s", event.Type, event.ID)
	return nil
}

// dispatch routes events to handlers based on type.
// For create/update events, we update synced_at on the object.
// For delete events, we soft-delete or hard-delete the object.
func (wh *WebhookHandler) dispatch(ctx ContextKey, event *StripeEvent, objectType, objectID string) error {
	eventType := event.Type

	// Handle delete events
	if isDeleteEvent(eventType) {
		return wh.db.DeleteObject(ctx, objectType, objectID)
	}

	// Handle payment_method.detached specially
	if eventType == "payment_method.detached" {
		_, err := wh.db.Pool.Exec(ctx,
			"UPDATE np_stripe_payment_methods SET customer_id = NULL, updated_at = NOW() WHERE id = $1 AND source_account_id = $2",
			objectID, wh.db.SourceAccountID,
		)
		return err
	}

	// Handle balance.available (informational only)
	if eventType == "balance.available" {
		log.Println("[stripe:webhooks] Balance available updated")
		return nil
	}

	// Handle invoice.upcoming (no ID)
	if eventType == "invoice.upcoming" {
		log.Println("[stripe:webhooks] Upcoming invoice notification received")
		return nil
	}

	// Handle payout events (informational)
	if strings.HasPrefix(eventType, "payout.") {
		log.Printf("[stripe:webhooks] Payout event: type=%s", eventType)
		return nil
	}

	// For all other create/update events, update synced_at
	if objectID != "" && objectType != "" {
		return wh.db.UpsertFromWebhookEvent(ctx, objectType, event.Data.Object)
	}

	return nil
}

// ContextKey is an alias for context.Context to avoid import cycle naming
type ContextKey = interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(key interface{}) interface{}
}

func isDeleteEvent(eventType string) bool {
	return strings.HasSuffix(eventType, ".deleted") ||
		eventType == "invoiceitem.deleted" ||
		eventType == "customer.tax_id.deleted"
}

func extractObjectInfo(data json.RawMessage) (objectType string, objectID string) {
	var obj struct {
		Object string `json:"object"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return "unknown", "unknown"
	}
	if obj.Object == "" {
		obj.Object = "unknown"
	}
	if obj.ID == "" {
		obj.ID = "unknown"
	}
	return obj.Object, obj.ID
}

// FindMatchingAccount finds which account's webhook secret matches the signature.
// Returns the account index, or -1 if no match.
func FindMatchingAccount(rawBody []byte, signatureHeader string, accounts []StripeAccountConfig) int {
	// Only check accounts that have a webhook secret configured
	hasSecrets := false
	for _, acc := range accounts {
		if acc.WebhookSecret != "" {
			hasSecrets = true
			break
		}
	}

	if !hasSecrets {
		// No webhook secrets configured, use primary (index 0)
		return 0
	}

	bestIdx := -1
	bestDist := int64(math.MaxInt64)

	for i, acc := range accounts {
		if acc.WebhookSecret == "" {
			continue
		}
		if err := VerifyStripeSignature(rawBody, signatureHeader, acc.WebhookSecret); err == nil {
			// If multiple match (unlikely), pick the first
			if i < len(accounts) && int64(i) < bestDist {
				bestIdx = i
				bestDist = int64(i)
			}
		}
	}

	return bestIdx
}
