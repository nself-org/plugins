package internal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// VerifySignature checks the HMAC-SHA256 signature from GitHub webhooks.
// The signature header looks like "sha256=<hex>".
func VerifySignature(payload []byte, signatureHeader, secret string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}

	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}

	sig, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

// WebhookHandler processes incoming GitHub webhook events.
type WebhookHandler struct {
	db       *DB
	handlers map[string]func(ctx context.Context, payload map[string]interface{}, action string) error
}

// NewWebhookHandler creates a handler with all default event handlers registered.
func NewWebhookHandler(db *DB) *WebhookHandler {
	wh := &WebhookHandler{
		db:       db,
		handlers: make(map[string]func(ctx context.Context, payload map[string]interface{}, action string) error),
	}
	wh.registerDefaults()
	return wh
}

func (wh *WebhookHandler) registerDefaults() {
	// Each handler simply stores the event. The TS version fetches fresh data
	// from the API on each webhook, but the Go port stores the raw event and
	// relies on the sync service for data enrichment.
	wh.handlers["push"] = wh.handleGeneric
	wh.handlers["pull_request"] = wh.handleGeneric
	wh.handlers["pull_request_review"] = wh.handleGeneric
	wh.handlers["issues"] = wh.handleGeneric
	wh.handlers["issue_comment"] = wh.handleGeneric
	wh.handlers["release"] = wh.handleGeneric
	wh.handlers["workflow_run"] = wh.handleGeneric
	wh.handlers["workflow_job"] = wh.handleGeneric
	wh.handlers["deployment"] = wh.handleGeneric
	wh.handlers["deployment_status"] = wh.handleGeneric
	wh.handlers["repository"] = wh.handleGeneric
	wh.handlers["create"] = wh.handleGeneric
	wh.handlers["delete"] = wh.handleGeneric
	wh.handlers["star"] = wh.handleGeneric
	wh.handlers["fork"] = wh.handleGeneric
	wh.handlers["branch_protection_rule"] = wh.handleGeneric
	wh.handlers["check_suite"] = wh.handleGeneric
	wh.handlers["check_run"] = wh.handleGeneric
	wh.handlers["label"] = wh.handleGeneric
	wh.handlers["milestone"] = wh.handleGeneric
	wh.handlers["team"] = wh.handleGeneric
	wh.handlers["member"] = wh.handleGeneric
	wh.handlers["pull_request_review_comment"] = wh.handleGeneric
	wh.handlers["commit_comment"] = wh.handleGeneric
}

// Handle processes a webhook delivery. It stores the event, dispatches to the
// appropriate handler, and marks the event processed.
func (wh *WebhookHandler) Handle(ctx context.Context, deliveryID, event string, payload map[string]interface{}) error {
	action, _ := payload["action"].(string)

	var repoID *int64
	var repoFullName *string
	var senderLogin *string

	if repo, ok := payload["repository"].(map[string]interface{}); ok {
		if id, ok := repo["id"].(float64); ok {
			rid := int64(id)
			repoID = &rid
		}
		if fn, ok := repo["full_name"].(string); ok {
			repoFullName = &fn
		}
	}
	if sender, ok := payload["sender"].(map[string]interface{}); ok {
		if login, ok := sender["login"].(string); ok {
			senderLogin = &login
		}
	}

	dataBytes, _ := json.Marshal(payload)
	rawData := json.RawMessage(dataBytes)

	evt := WebhookEvent{
		ID:              deliveryID,
		SourceAccountID: wh.db.SourceAccountID,
		Event:           event,
		Action:          nilIfEmpty(action),
		RepoID:          repoID,
		RepoFullName:    repoFullName,
		SenderLogin:     senderLogin,
		Data:            &rawData,
		Processed:       false,
	}

	if err := wh.db.InsertWebhookEvent(ctx, evt); err != nil {
		log.Printf("[github:webhooks] Failed to store event: %v", err)
	}

	log.Printf("[github:webhooks] Event received: %s action=%s delivery=%s", event, action, deliveryID)

	handler, ok := wh.handlers[event]
	if ok {
		if err := handler(ctx, payload, action); err != nil {
			errMsg := err.Error()
			_ = wh.db.MarkEventProcessed(ctx, deliveryID, &errMsg)
			log.Printf("[github:webhooks] Event processing failed: %s %s %v", event, deliveryID, err)
			return fmt.Errorf("webhook handler failed for %s: %w", event, err)
		}
		_ = wh.db.MarkEventProcessed(ctx, deliveryID, nil)
		log.Printf("[github:webhooks] Event processed: %s %s", event, deliveryID)
	} else {
		_ = wh.db.MarkEventProcessed(ctx, deliveryID, nil)
		log.Printf("[github:webhooks] No handler for event: %s", event)
	}

	return nil
}

// handleGeneric is the default handler that simply logs the event.
// The event is already stored by Handle before this is called.
func (wh *WebhookHandler) handleGeneric(_ context.Context, payload map[string]interface{}, action string) error {
	repo := "unknown"
	if r, ok := payload["repository"].(map[string]interface{}); ok {
		if fn, ok := r["full_name"].(string); ok {
			repo = fn
		}
	}
	log.Printf("[github:webhooks] Processed generic event for %s (action=%s)", repo, action)
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
