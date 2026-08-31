// Package paddle implements the Provider interface for Paddle.
package paddle

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/nself-payments/internal/provider"
)

const (
	baseURL = "https://api.paddle.com"
)

// Adapter is the Paddle provider implementation.
type Adapter struct {
	apiKey    string
	client    *http.Client
	publicKey *rsa.PublicKey
}

// New creates a new Paddle adapter.
func New(apiKey, publicKeyPEM string) (*Adapter, error) {
	a := &Adapter{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	if publicKeyPEM != "" {
		pk, err := parseRSAPublicKey(publicKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("paddle: parse public key: %w", err)
		}
		a.publicKey = pk
	}
	return a, nil
}

func (a *Adapter) Name() string { return "paddle" }

func (a *Adapter) do(ctx context.Context, method, path string, body io.Reader, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return fmt.Errorf("paddle: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("paddle: http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("paddle: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("paddle: API error %d: %s", resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (a *Adapter) CreateCheckout(ctx context.Context, params provider.CheckoutParams) (*provider.CheckoutResult, error) {
	payload := map[string]interface{}{
		"items": []map[string]interface{}{
			{"price_id": params.PlanID, "quantity": 1},
		},
		"customer_email": params.Email,
		"success_url":    params.SuccessURL,
		"custom_data": map[string]string{
			"user_id": params.UserID,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("paddle: marshal checkout: %w", err)
	}
	var result struct {
		Data struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodPost, "/transactions", bytes.NewReader(b), &result); err != nil {
		return nil, err
	}
	return &provider.CheckoutResult{URL: result.Data.URL, SessionID: result.Data.ID}, nil
}

func (a *Adapter) GetSubscription(ctx context.Context, providerSubID string) (*provider.Subscription, error) {
	var result struct {
		Data paddleSubscription `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, "/subscriptions/"+providerSubID, nil, &result); err != nil {
		return nil, err
	}
	return result.Data.toCanonical(), nil
}

func (a *Adapter) CreatePortalURL(ctx context.Context, customerID string, returnURL string) (*provider.PortalResult, error) {
	payload := map[string]interface{}{
		"return_url": returnURL,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("paddle: marshal portal: %w", err)
	}
	var result struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodPost, "/customers/"+customerID+"/auth-token", bytes.NewReader(b), &result); err != nil {
		return nil, err
	}
	return &provider.PortalResult{URL: result.Data.URL}, nil
}

func (a *Adapter) CancelSubscription(ctx context.Context, providerSubID string) error {
	payload := map[string]interface{}{"effective_from": "next_billing_period"}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("paddle: marshal cancel: %w", err)
	}
	return a.do(ctx, http.MethodPost, "/subscriptions/"+providerSubID+"/cancel", bytes.NewReader(b), nil)
}

func (a *Adapter) PauseSubscription(ctx context.Context, providerSubID string) error {
	payload := map[string]interface{}{"effective_from": "next_billing_period"}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("paddle: marshal pause: %w", err)
	}
	return a.do(ctx, http.MethodPost, "/subscriptions/"+providerSubID+"/pause", bytes.NewReader(b), nil)
}

func (a *Adapter) RetryInvoice(ctx context.Context, providerSubID string) (*provider.RetryResult, error) {
	payload := map[string]interface{}{"effective_from": "immediately"}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("paddle: marshal retry: %w", err)
	}
	if err := a.do(ctx, http.MethodPost, "/subscriptions/"+providerSubID+"/resume", bytes.NewReader(b), nil); err != nil {
		return &provider.RetryResult{Success: false, Error: err.Error()}, nil
	}
	return &provider.RetryResult{Success: true}, nil
}

func (a *Adapter) ListPlans(ctx context.Context) ([]provider.Plan, error) {
	var result struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			BillingCycle struct {
				Interval string `json:"interval"`
			} `json:"billing_cycle"`
			UnitPrice struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency_code"`
			} `json:"unit_price"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, "/prices", nil, &result); err != nil {
		return nil, err
	}
	plans := make([]provider.Plan, 0, len(result.Data))
	for _, p := range result.Data {
		plans = append(plans, provider.Plan{
			ID:       p.ID,
			Name:     p.Name,
			Currency: strings.ToLower(p.UnitPrice.Currency),
			Interval: p.BillingCycle.Interval,
		})
	}
	return plans, nil
}

func (a *Adapter) RecordUsage(_ context.Context, _ provider.UsageRecord) error {
	// Paddle usage billing is handled through subscription quantity updates.
	// Usage records are stored in np_usage_records only.
	return nil
}

// VerifyWebhookSignature validates Paddle's public-key RSA-SHA1 signature.
func (a *Adapter) VerifyWebhookSignature(signature string, body []byte, _ string) (string, error) {
	if a.publicKey == nil {
		return "", fmt.Errorf("paddle: public key not configured (PADDLE_PUBLIC_KEY)")
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("paddle: parse webhook body: %w", err)
	}

	sig := vals.Get("p_signature")
	if sig == "" {
		sig = signature
	}
	vals.Del("p_signature")

	// Sort keys and build serialized payload (Paddle PHP serialize scheme).
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		v := vals.Get(k)
		sb.WriteString(phpSerializeString(v))
	}

	hash := sha1.Sum([]byte(sb.String()))
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return "", fmt.Errorf("paddle: decode signature base64: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(a.publicKey, crypto.SHA1, hash[:], sigBytes); err != nil {
		return "", fmt.Errorf("paddle: signature verification failed: %w", err)
	}

	return vals.Get("alert_name"), nil
}

func (a *Adapter) ParseWebhookEvent(eventType string, body []byte) (*provider.Subscription, error) {
	subEvents := map[string]bool{
		"subscription_created":           true,
		"subscription_updated":           true,
		"subscription_cancelled":         true,
		"subscription_payment_failed":    true,
		"subscription_payment_succeeded": true,
	}
	if !subEvents[eventType] {
		return nil, nil
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("paddle: parse webhook: %w", err)
	}

	status := provider.StatusActive
	switch eventType {
	case "subscription_cancelled":
		status = provider.StatusCanceled
	case "subscription_payment_failed":
		status = provider.StatusPastDue
	}

	sub := &provider.Subscription{
		ProviderSubID:      vals.Get("subscription_id"),
		ProviderCustomerID: vals.Get("user_id"),
		PlanID:             vals.Get("subscription_plan_id"),
		Status:             status,
	}
	if nextBill := vals.Get("next_bill_date"); nextBill != "" {
		t, err := time.Parse("2006-01-02", nextBill)
		if err == nil {
			sub.CurrentPeriodEnd = t
		}
	}
	return sub, nil
}

type paddleSubscription struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Status     string `json:"status"`
	Items      []struct {
		Price struct {
			ID string `json:"id"`
		} `json:"price"`
	} `json:"items"`
	CurrentBillingPeriod struct {
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	} `json:"current_billing_period"`
}

func (p *paddleSubscription) toCanonical() *provider.Subscription {
	sub := &provider.Subscription{
		ProviderSubID:      p.ID,
		ProviderCustomerID: p.CustomerID,
		Status:             mapPaddleStatus(p.Status),
	}
	if len(p.Items) > 0 {
		sub.PlanID = p.Items[0].Price.ID
	}
	if s := p.CurrentBillingPeriod.StartsAt; s != "" {
		t, _ := time.Parse(time.RFC3339, s)
		sub.CurrentPeriodStart = t
	}
	if e := p.CurrentBillingPeriod.EndsAt; e != "" {
		t, _ := time.Parse(time.RFC3339, e)
		sub.CurrentPeriodEnd = t
	}
	return sub
}

func mapPaddleStatus(s string) provider.SubscriptionStatus {
	switch s {
	case "active":
		return provider.StatusActive
	case "past_due":
		return provider.StatusPastDue
	case "canceled":
		return provider.StatusCanceled
	case "paused":
		return provider.StatusPaused
	case "trialing":
		return provider.StatusTrialing
	default:
		return provider.SubscriptionStatus(s)
	}
}

func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rk, nil
}

func phpSerializeString(s string) string {
	return fmt.Sprintf("s:%d:\"%s\";", len(s), s)
}
