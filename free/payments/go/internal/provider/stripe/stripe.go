// Package stripe implements the Provider interface for Stripe.
package stripe

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

	"github.com/google/uuid"
	"github.com/nself-org/nself-payments/internal/provider"
)

// retryIdempotencyKey derives a stable Idempotency-Key for a RetryInvoice call.
// The key is bound to the invoice ID and the minute bucket so that:
//   - Multiple retries of the same invoice within the same minute reuse the key
//     (Stripe deduplicates → no duplicate charge).
//   - Retries in a later minute get a fresh key (allowing a genuine new attempt).
//
// Format: "<invoiceID>-retry-<minute-epoch>"
func retryIdempotencyKey(invoiceID string) string {
	minuteBucket := time.Now().UTC().Unix() / 60
	return fmt.Sprintf("%s-retry-%d", invoiceID, minuteBucket)
}

const (
	baseURL       = "https://api.stripe.com/v1"
	apiVersion    = "2024-06-20"
	webhookTolSec = 300 // 5 minutes replay protection
)

// Adapter is the Stripe provider implementation.
type Adapter struct {
	secretKey string
	client    *http.Client
}

// New creates a new Stripe adapter.
func New(secretKey string) *Adapter {
	return &Adapter{
		secretKey: secretKey,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string { return "stripe" }

// do executes a Stripe API request and decodes the JSON response.
func (a *Adapter) do(ctx context.Context, method, path string, form url.Values, out interface{}) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(a.secretKey, "")
	req.Header.Set("Stripe-Version", apiVersion)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("stripe: http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("stripe: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("stripe: API error %d: %s", resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// doWithIdempotency executes a Stripe API request with an Idempotency-Key header.
// Use this for mutating requests where duplicate calls must not create duplicate resources.
func (a *Adapter) doWithIdempotency(ctx context.Context, method, path, idempotencyKey string, form url.Values, out interface{}) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(a.secretKey, "")
	req.Header.Set("Stripe-Version", apiVersion)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("stripe: http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("stripe: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("stripe: API error %d: %s", resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (a *Adapter) CreateCheckout(ctx context.Context, params provider.CheckoutParams) (*provider.CheckoutResult, error) {
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("success_url", params.SuccessURL)
	form.Set("cancel_url", params.CancelURL)
	form.Set("line_items[0][price]", params.PlanID)
	form.Set("line_items[0][quantity]", "1")
	if params.Email != "" {
		form.Set("customer_email", params.Email)
	}
	if params.UserID != "" {
		form.Set("metadata[user_id]", params.UserID)
	}

	// Derive the Idempotency-Key. Stripe deduplicates POST /checkout/sessions
	// within a 24-hour window when the same key is presented, so retries from
	// network blips or queue re-deliveries cannot produce a double-charge.
	// Use the caller-supplied RequestUUID (from X-Request-Id header) when
	// available; generate a fresh UUID v4 otherwise.
	idempotencyKey := params.RequestUUID
	if idempotencyKey == "" {
		idempotencyKey = uuid.New().String()
	}

	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := a.doWithIdempotency(ctx, http.MethodPost, "/checkout/sessions", idempotencyKey, form, &result); err != nil {
		return nil, err
	}
	return &provider.CheckoutResult{URL: result.URL, SessionID: result.ID}, nil
}

func (a *Adapter) GetSubscription(ctx context.Context, providerSubID string) (*provider.Subscription, error) {
	var result stripeSubscription
	if err := a.do(ctx, http.MethodGet, "/subscriptions/"+providerSubID, nil, &result); err != nil {
		return nil, err
	}
	return result.toCanonical(), nil
}

func (a *Adapter) CreatePortalURL(ctx context.Context, customerID string, returnURL string) (*provider.PortalResult, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	if returnURL != "" {
		form.Set("return_url", returnURL)
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := a.do(ctx, http.MethodPost, "/billing_portal/sessions", form, &result); err != nil {
		return nil, err
	}
	return &provider.PortalResult{URL: result.URL}, nil
}

func (a *Adapter) CancelSubscription(ctx context.Context, providerSubID string) error {
	form := url.Values{}
	form.Set("cancel_at_period_end", "true")
	return a.do(ctx, http.MethodPost, "/subscriptions/"+providerSubID, form, nil)
}

func (a *Adapter) PauseSubscription(_ context.Context, _ string) error {
	return fmt.Errorf("stripe: pause not natively supported; use cancel_at_period_end instead")
}

func (a *Adapter) RetryInvoice(ctx context.Context, providerSubID string) (*provider.RetryResult, error) {
	// Find the latest open invoice for the subscription.
	form := url.Values{}
	form.Set("subscription", providerSubID)
	form.Set("status", "open")
	form.Set("limit", "1")

	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, "/invoices?"+form.Encode(), nil, &list); err != nil {
		return nil, err
	}
	if len(list.Data) == 0 {
		return &provider.RetryResult{Success: false, Error: "no open invoice found"}, nil
	}

	invoiceID := list.Data[0].ID

	// Derive a stable idempotency key from the invoice ID + minute bucket.
	// Stripe deduplicates within 24h when the same key is reused, preventing
	// duplicate payment attempts on network retry. Callers retrying within the
	// same minute get the same key; a later-minute retry gets a fresh key so a
	// genuine new payment attempt is possible after the window expires.
	idemKey := retryIdempotencyKey(invoiceID)

	if err := a.doWithIdempotency(ctx, http.MethodPost, "/invoices/"+invoiceID+"/pay", idemKey, nil, nil); err != nil {
		return &provider.RetryResult{Success: false, Error: err.Error()}, nil
	}
	return &provider.RetryResult{Success: true}, nil
}

func (a *Adapter) ListPlans(ctx context.Context) ([]provider.Plan, error) {
	var result struct {
		Data []struct {
			ID       string `json:"id"`
			Nickname string `json:"nickname"`
			UnitAmount int64 `json:"unit_amount"`
			Currency string `json:"currency"`
			Recurring struct {
				Interval string `json:"interval"`
			} `json:"recurring"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, "/prices?active=true&type=recurring&limit=100", nil, &result); err != nil {
		return nil, err
	}
	plans := make([]provider.Plan, 0, len(result.Data))
	for _, p := range result.Data {
		plans = append(plans, provider.Plan{
			ID:       p.ID,
			Name:     p.Nickname,
			Price:    p.UnitAmount,
			Currency: p.Currency,
			Interval: p.Recurring.Interval,
		})
	}
	return plans, nil
}

func (a *Adapter) RecordUsage(ctx context.Context, rec provider.UsageRecord) error {
	form := url.Values{}
	form.Set("quantity", strconv.Itoa(rec.Quantity))
	form.Set("timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	form.Set("action", "increment")
	path := fmt.Sprintf("/subscription_items/%s/usage_records", rec.MetricID)
	return a.do(ctx, http.MethodPost, path, form, nil)
}

// VerifyWebhookSignature validates the Stripe-Signature header using HMAC-SHA256.
// This uses the raw body — must be called before JSON parsing.
func (a *Adapter) VerifyWebhookSignature(signature string, body []byte, secret string) (string, error) {
	// Stripe signature format: t=<timestamp>,v1=<hex-sig>,...
	parts := strings.Split(signature, ",")
	var ts string
	var sig string
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			if sig == "" {
				sig = kv[1]
			}
		}
	}
	if ts == "" || sig == "" {
		return "", fmt.Errorf("stripe: malformed Stripe-Signature header")
	}

	// Replay protection: timestamp must be within ±5 minutes.
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return "", fmt.Errorf("stripe: invalid timestamp in signature")
	}
	if diff := time.Now().Unix() - tsInt; diff > webhookTolSec || diff < -webhookTolSec {
		return "", fmt.Errorf("stripe: webhook timestamp outside tolerance (%d seconds)", diff)
	}

	// Compute expected HMAC-SHA256.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return "", fmt.Errorf("stripe: webhook signature mismatch")
	}

	// Extract event type from raw body without full parse.
	var event struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&event); err != nil {
		return "", fmt.Errorf("stripe: decode event type: %w", err)
	}
	return event.Type, nil
}

func (a *Adapter) ParseWebhookEvent(eventType string, body []byte) (*provider.Subscription, error) {
	// Events that carry subscription data.
	subEvents := map[string]bool{
		"customer.subscription.created":          true,
		"customer.subscription.updated":          true,
		"customer.subscription.deleted":          true,
		"invoice.payment_succeeded":              true,
		"invoice.payment_failed":                 true,
	}
	if !subEvents[eventType] {
		return nil, nil // non-subscription event — caller should acknowledge
	}

	var envelope struct {
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("stripe: parse event envelope: %w", err)
	}

	// For invoice events the object is an invoice; fetch subscription from it.
	if strings.HasPrefix(eventType, "invoice.") {
		var invoice struct {
			SubscriptionID string `json:"subscription"`
			CustomerID     string `json:"customer"`
		}
		if err := json.Unmarshal(envelope.Data.Object, &invoice); err != nil {
			return nil, fmt.Errorf("stripe: parse invoice: %w", err)
		}
		status := StatusActive
		if eventType == "invoice.payment_failed" {
			status = StatusPastDue
		}
		return &provider.Subscription{
			ProviderSubID:      invoice.SubscriptionID,
			ProviderCustomerID: invoice.CustomerID,
			Status:             provider.SubscriptionStatus(status),
		}, nil
	}

	// Subscription object events.
	var sub stripeSubscription
	if err := json.Unmarshal(envelope.Data.Object, &sub); err != nil {
		return nil, fmt.Errorf("stripe: parse subscription: %w", err)
	}
	return sub.toCanonical(), nil
}

// StatusActive etc. as local consts to avoid repeated provider.Status* references.
const (
	StatusActive   = "active"
	StatusPastDue  = "past_due"
	StatusCanceled = "canceled"
	StatusPaused   = "paused"
	StatusTrialing = "trialing"
)

type stripeSubscription struct {
	ID                 string `json:"id"`
	CustomerID         string `json:"customer"`
	Status             string `json:"status"`
	CurrentPeriodStart int64  `json:"current_period_start"`
	CurrentPeriodEnd   int64  `json:"current_period_end"`
	CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
	TrialEnd           *int64 `json:"trial_end"`
	Items              struct {
		Data []struct {
			Price struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
	Metadata map[string]string `json:"metadata"`
}

func (s *stripeSubscription) toCanonical() *provider.Subscription {
	sub := &provider.Subscription{
		ProviderSubID:      s.ID,
		ProviderCustomerID: s.CustomerID,
		Status:             provider.SubscriptionStatus(s.Status),
		CurrentPeriodStart: time.Unix(s.CurrentPeriodStart, 0).UTC(),
		CurrentPeriodEnd:   time.Unix(s.CurrentPeriodEnd, 0).UTC(),
		CancelAtPeriodEnd:  s.CancelAtPeriodEnd,
		Metadata:           s.Metadata,
	}
	if s.TrialEnd != nil {
		t := time.Unix(*s.TrialEnd, 0).UTC()
		sub.TrialEnd = &t
	}
	if len(s.Items.Data) > 0 {
		sub.PlanID = s.Items.Data[0].Price.ID
	}
	return sub
}
