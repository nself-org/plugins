// Package lemonsqueezy implements the Provider interface for Lemon Squeezy.
package lemonsqueezy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/nself-payments/internal/provider"
)

const (
	baseURL       = "https://api.lemonsqueezy.com/v1"
	webhookTolSec = 300 // 5 minutes replay protection
)

// Adapter is the Lemon Squeezy provider implementation.
type Adapter struct {
	apiKey  string
	storeID string
	client  *http.Client
}

// New creates a new Lemon Squeezy adapter.
func New(apiKey, storeID string) *Adapter {
	return &Adapter{
		apiKey:  apiKey,
		storeID: storeID,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string { return "lemonsqueezy" }

func (a *Adapter) do(ctx context.Context, method, path string, body io.Reader, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return fmt.Errorf("lemonsqueezy: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Accept", "application/vnd.api+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("lemonsqueezy: http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("lemonsqueezy: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("lemonsqueezy: API error %d: %s", resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (a *Adapter) CreateCheckout(ctx context.Context, params provider.CheckoutParams) (*provider.CheckoutResult, error) {
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "checkouts",
			"attributes": map[string]interface{}{
				"checkout_data": map[string]interface{}{
					"email": params.Email,
					"custom": map[string]string{
						"user_id": params.UserID,
					},
				},
				"product_options": map[string]interface{}{
					"redirect_url":    params.SuccessURL,
					"receipt_link_url": params.SuccessURL,
				},
			},
			"relationships": map[string]interface{}{
				"store": map[string]interface{}{
					"data": map[string]string{"type": "stores", "id": a.storeID},
				},
				"variant": map[string]interface{}{
					"data": map[string]string{"type": "variants", "id": params.PlanID},
				},
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("lemonsqueezy: marshal checkout: %w", err)
	}

	var result struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				URL string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodPost, "/checkouts", bytes.NewReader(b), &result); err != nil {
		return nil, err
	}
	return &provider.CheckoutResult{
		URL:       result.Data.Attributes.URL,
		SessionID: result.Data.ID,
	}, nil
}

func (a *Adapter) GetSubscription(ctx context.Context, providerSubID string) (*provider.Subscription, error) {
	var result lsSubscriptionResponse
	if err := a.do(ctx, http.MethodGet, "/subscriptions/"+providerSubID, nil, &result); err != nil {
		return nil, err
	}
	return result.toCanonical()
}

func (a *Adapter) CreatePortalURL(ctx context.Context, customerID string, _ string) (*provider.PortalResult, error) {
	// LS: use the customer portal URL directly (returned in subscription response).
	var result struct {
		Data struct {
			Attributes struct {
				URLs struct {
					CustomerPortal string `json:"customer_portal"`
				} `json:"urls"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, "/subscriptions/"+customerID, nil, &result); err != nil {
		return nil, err
	}
	return &provider.PortalResult{URL: result.Data.Attributes.URLs.CustomerPortal}, nil
}

func (a *Adapter) CancelSubscription(ctx context.Context, providerSubID string) error {
	return a.do(ctx, http.MethodDelete, "/subscriptions/"+providerSubID, nil, nil)
}

func (a *Adapter) PauseSubscription(ctx context.Context, providerSubID string) error {
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "subscriptions",
			"id":   providerSubID,
			"attributes": map[string]interface{}{
				"pause": map[string]string{"mode": "void"},
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("lemonsqueezy: marshal pause: %w", err)
	}
	return a.do(ctx, http.MethodPatch, "/subscriptions/"+providerSubID, bytes.NewReader(b), nil)
}

func (a *Adapter) RetryInvoice(ctx context.Context, providerSubID string) (*provider.RetryResult, error) {
	// LS does not expose a direct invoice-retry API; resume the subscription instead.
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "subscriptions",
			"id":   providerSubID,
			"attributes": map[string]interface{}{
				"pause": nil, // null clears pause/cancellation
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("lemonsqueezy: marshal retry: %w", err)
	}
	if err := a.do(ctx, http.MethodPatch, "/subscriptions/"+providerSubID, bytes.NewReader(b), nil); err != nil {
		return &provider.RetryResult{Success: false, Error: err.Error()}, nil
	}
	return &provider.RetryResult{Success: true}, nil
}

func (a *Adapter) ListPlans(ctx context.Context) ([]provider.Plan, error) {
	path := "/variants?filter[store_id]=" + url.QueryEscape(a.storeID)
	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name     string `json:"name"`
				Price    int64  `json:"price"`
				Interval string `json:"interval"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	plans := make([]provider.Plan, 0, len(result.Data))
	for _, v := range result.Data {
		plans = append(plans, provider.Plan{
			ID:       v.ID,
			Name:     v.Attributes.Name,
			Price:    v.Attributes.Price,
			Currency: "usd",
			Interval: v.Attributes.Interval,
		})
	}
	return plans, nil
}

func (a *Adapter) RecordUsage(_ context.Context, _ provider.UsageRecord) error {
	// Lemon Squeezy does not natively support usage-based billing per metric.
	// Usage records are stored in np_usage_records only.
	return nil
}

// VerifyWebhookSignature validates the X-Signature header using HMAC-SHA256.
func (a *Adapter) VerifyWebhookSignature(signature string, body []byte, secret string) (string, error) {
	if signature == "" {
		return "", fmt.Errorf("lemonsqueezy: missing X-Signature header")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return "", fmt.Errorf("lemonsqueezy: webhook signature mismatch")
	}

	// Extract event name from meta.
	var meta struct {
		Meta struct {
			EventName string `json:"event_name"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", fmt.Errorf("lemonsqueezy: decode event name: %w", err)
	}
	return meta.Meta.EventName, nil
}

func (a *Adapter) ParseWebhookEvent(eventType string, body []byte) (*provider.Subscription, error) {
	subEvents := map[string]bool{
		"subscription_created":       true,
		"subscription_updated":       true,
		"subscription_cancelled":     true,
		"subscription_resumed":       true,
		"subscription_payment_failed": true,
		"subscription_expired":       true,
	}
	if !subEvents[eventType] {
		return nil, nil
	}

	var envelope struct {
		Data lsSubscriptionData `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("lemonsqueezy: parse webhook: %w", err)
	}
	resp := lsSubscriptionResponse{Data: envelope.Data}
	return resp.toCanonical()
}

type lsSubscriptionData struct {
	ID         string `json:"id"`
	Attributes struct {
		CustomerID       string `json:"customer_id"`
		ProductID        string `json:"product_id"`
		VariantID        string `json:"variant_id"`
		Status           string `json:"status"`
		RenewsAt         string `json:"renews_at"`
		CreatedAt        string `json:"created_at"`
		TrialEndsAt      string `json:"trial_ends_at"`
		EndsAt           string `json:"ends_at"`
		CancelledAt      string `json:"cancelled_at"`
	} `json:"attributes"`
}

type lsSubscriptionResponse struct {
	Data lsSubscriptionData `json:"data"`
}

func (r *lsSubscriptionResponse) toCanonical() (*provider.Subscription, error) {
	attr := r.Data.Attributes
	status := mapLSStatus(attr.Status)

	sub := &provider.Subscription{
		ProviderSubID:      r.Data.ID,
		ProviderCustomerID: strconv.Itoa(mustInt(attr.CustomerID)),
		PlanID:             attr.VariantID,
		Status:             status,
	}

	if attr.RenewsAt != "" {
		t, err := time.Parse(time.RFC3339, attr.RenewsAt)
		if err == nil {
			sub.CurrentPeriodEnd = t
		}
	}
	if attr.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, attr.CreatedAt)
		if err == nil {
			sub.CurrentPeriodStart = t
		}
	}
	if attr.TrialEndsAt != "" {
		t, err := time.Parse(time.RFC3339, attr.TrialEndsAt)
		if err == nil {
			sub.TrialEnd = &t
		}
	}
	if strings.Contains(attr.Status, "cancelled") || attr.CancelledAt != "" {
		sub.CancelAtPeriodEnd = true
	}
	return sub, nil
}

func mapLSStatus(s string) provider.SubscriptionStatus {
	switch s {
	case "active":
		return provider.StatusActive
	case "past_due":
		return provider.StatusPastDue
	case "cancelled", "expired":
		return provider.StatusCanceled
	case "paused":
		return provider.StatusPaused
	case "on_trial":
		return provider.StatusTrialing
	default:
		return provider.SubscriptionStatus(s)
	}
}

func mustInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
