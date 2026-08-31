// Package stripe provides a minimal Stripe API client for nself-cloud provisioning.
// Only charge and refund operations are implemented here; full webhook processing
// lives in S6.T09.
package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const stripeBaseURL = "https://api.stripe.com/v1"

// Client is a minimal Stripe API client.
type Client struct {
	secretKey  string
	httpClient *http.Client
}

// New creates a Client using STRIPE_PLATFORM_SECRET_KEY from env.
func New() (*Client, error) {
	key := os.Getenv("STRIPE_PLATFORM_SECRET_KEY")
	if key == "" {
		return nil, fmt.Errorf("stripe: STRIPE_PLATFORM_SECRET_KEY not set")
	}
	return &Client{
		secretKey: key,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// NewWithKey creates a Client with an explicit secret key (useful for tests).
func NewWithKey(key string) *Client {
	return &Client{
		secretKey: key,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ChargeResult is returned on a successful charge.
type ChargeResult struct {
	ChargeID string
	Paid     bool
}

// Charge creates a Stripe charge for the given customer.
// idempotencyKey must be unique per (instanceID) to allow safe retries.
func (c *Client) Charge(ctx context.Context, customerID string, amountCents int, currency, idempotencyKey, description string) (ChargeResult, error) {
	params := url.Values{}
	params.Set("amount", fmt.Sprintf("%d", amountCents))
	params.Set("currency", currency)
	params.Set("customer", customerID)
	params.Set("description", description)

	resp, err := c.doForm(ctx, http.MethodPost, "/charges", params, idempotencyKey)
	if err != nil {
		return ChargeResult{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("stripe: read charge response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ChargeResult{}, fmt.Errorf("stripe: charge HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		ID   string `json:"id"`
		Paid bool   `json:"paid"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChargeResult{}, fmt.Errorf("stripe: parse charge response: %w", err)
	}
	return ChargeResult{ChargeID: parsed.ID, Paid: parsed.Paid}, nil
}

// Refund issues a full refund for chargeID.
// idempotencyKey must be unique per refund attempt.
func (c *Client) Refund(ctx context.Context, chargeID, idempotencyKey string) error {
	params := url.Values{}
	params.Set("charge", chargeID)

	resp, err := c.doForm(ctx, http.MethodPost, "/refunds", params, idempotencyKey)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("stripe: read refund response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stripe: refund HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// doForm executes a form-encoded Stripe API request.
func (c *Client) doForm(ctx context.Context, method, path string, params url.Values, idempotencyKey string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, stripeBaseURL+path,
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(c.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return c.httpClient.Do(req)
}
