package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StripeClient makes authenticated requests to the Stripe API.
type StripeClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewStripeClient creates a client with the given API key.
func NewStripeClient(apiKey string) *StripeClient {
	return &StripeClient{
		apiKey:  apiKey,
		baseURL: "https://api.stripe.com/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithAPIKey returns a new client using a different API key (for multi-account).
func (c *StripeClient) WithAPIKey(apiKey string) *StripeClient {
	return &StripeClient{
		apiKey:     apiKey,
		baseURL:    c.baseURL,
		httpClient: c.httpClient,
	}
}

// stripeListResponse is the generic Stripe list response envelope.
type stripeListResponse struct {
	Object  string            `json:"object"`
	Data    []json.RawMessage `json:"data"`
	HasMore bool              `json:"has_more"`
	URL     string            `json:"url"`
}

// get makes an authenticated GET request to a Stripe API endpoint.
func (c *StripeClient) get(path string, params url.Values) ([]byte, error) {
	reqURL := c.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe API error %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	return body, nil
}

// post makes an authenticated POST request with form-encoded body.
func (c *StripeClient) post(path string, params url.Values) ([]byte, error) {
	reqURL := c.baseURL + path

	var bodyReader io.Reader
	if len(params) > 0 {
		bodyReader = strings.NewReader(params.Encode())
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe API error %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	return body, nil
}

// listAll paginates through all pages of a Stripe list endpoint.
func (c *StripeClient) listAll(path string, extraParams url.Values) ([]json.RawMessage, error) {
	var all []json.RawMessage
	params := url.Values{}
	params.Set("limit", "100")
	for k, v := range extraParams {
		params[k] = v
	}

	for {
		body, err := c.get(path, params)
		if err != nil {
			return all, err
		}

		var resp stripeListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return all, fmt.Errorf("parse list response: %w", err)
		}

		all = append(all, resp.Data...)

		if !resp.HasMore || len(resp.Data) == 0 {
			break
		}

		// Extract the last object's ID for pagination
		var lastObj struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp.Data[len(resp.Data)-1], &lastObj); err != nil {
			return all, fmt.Errorf("extract last id: %w", err)
		}
		params.Set("starting_after", lastObj.ID)
	}

	return all, nil
}

// --- Public list methods ---

func (c *StripeClient) ListCustomers() ([]json.RawMessage, error) {
	return c.listAll("/customers", nil)
}

func (c *StripeClient) GetCustomer(id string) (json.RawMessage, error) {
	body, err := c.get("/customers/"+id, nil)
	return json.RawMessage(body), err
}

func (c *StripeClient) ListProducts() ([]json.RawMessage, error) {
	return c.listAll("/products", nil)
}

func (c *StripeClient) ListPrices() ([]json.RawMessage, error) {
	return c.listAll("/prices", nil)
}

func (c *StripeClient) ListCoupons() ([]json.RawMessage, error) {
	return c.listAll("/coupons", nil)
}

func (c *StripeClient) ListPromotionCodes() ([]json.RawMessage, error) {
	return c.listAll("/promotion_codes", nil)
}

func (c *StripeClient) ListSubscriptions() ([]json.RawMessage, error) {
	params := url.Values{}
	params.Set("status", "all")
	return c.listAll("/subscriptions", params)
}

func (c *StripeClient) ListSubscriptionItems(subscriptionID string) ([]json.RawMessage, error) {
	params := url.Values{}
	params.Set("subscription", subscriptionID)
	return c.listAll("/subscription_items", params)
}

func (c *StripeClient) ListInvoices() ([]json.RawMessage, error) {
	return c.listAll("/invoices", nil)
}

func (c *StripeClient) ListInvoiceItems(invoiceID string) ([]json.RawMessage, error) {
	params := url.Values{}
	params.Set("invoice", invoiceID)
	return c.listAll("/invoiceitems", params)
}

func (c *StripeClient) ListCharges() ([]json.RawMessage, error) {
	return c.listAll("/charges", nil)
}

func (c *StripeClient) ListRefunds() ([]json.RawMessage, error) {
	return c.listAll("/refunds", nil)
}

func (c *StripeClient) ListDisputes() ([]json.RawMessage, error) {
	return c.listAll("/disputes", nil)
}

func (c *StripeClient) ListPaymentIntents() ([]json.RawMessage, error) {
	return c.listAll("/payment_intents", nil)
}

func (c *StripeClient) ListSetupIntents() ([]json.RawMessage, error) {
	return c.listAll("/setup_intents", nil)
}

func (c *StripeClient) ListPaymentMethods(customerID string) ([]json.RawMessage, error) {
	params := url.Values{}
	params.Set("customer", customerID)
	params.Set("type", "card")
	return c.listAll("/payment_methods", params)
}

func (c *StripeClient) ListBalanceTransactions() ([]json.RawMessage, error) {
	return c.listAll("/balance_transactions", nil)
}

func (c *StripeClient) ListCheckoutSessions() ([]json.RawMessage, error) {
	return c.listAll("/checkout/sessions", nil)
}

func (c *StripeClient) ListTaxRates() ([]json.RawMessage, error) {
	return c.listAll("/tax_rates", nil)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
