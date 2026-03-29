package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// PayPalClient handles OAuth2 authentication and API calls to PayPal.
type PayPalClient struct {
	config      *Config
	baseURL     string
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
	mu          sync.Mutex
}

// NewPayPalClient creates a new PayPal API client.
func NewPayPalClient(cfg *Config) *PayPalClient {
	return &PayPalClient{
		config:  cfg,
		baseURL: cfg.BaseURL(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// tokenResponse represents the OAuth2 token response from PayPal.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getAccessToken returns a cached token or fetches a new one using client credentials.
func (c *PayPalClient) getAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-5*time.Minute)) {
		return c.accessToken, nil
	}

	creds := base64.StdEncoding.EncodeToString([]byte(c.config.ClientID + ":" + c.config.ClientSecret))

	req, err := http.NewRequest("POST", c.baseURL+"/v1/oauth2/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", fmt.Errorf("paypal oauth2: create request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal oauth2: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("paypal oauth2: status %d: %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("paypal oauth2: decode response: %w", err)
	}

	c.accessToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)

	return c.accessToken, nil
}

// doRequest performs an authenticated HTTP request to the PayPal API.
func (c *PayPalClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("paypal api: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal api: request failed: %w", err)
	}

	// Retry once on 401 with a fresh token.
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.mu.Lock()
		c.accessToken = ""
		c.mu.Unlock()

		token, err = c.getAccessToken()
		if err != nil {
			return nil, err
		}

		req, err = http.NewRequest(method, c.baseURL+path, body)
		if err != nil {
			return nil, fmt.Errorf("paypal api: retry create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("paypal api: retry request failed: %w", err)
		}
	}

	return resp, nil
}

// --- Transaction Search (31-day windowing) -----------------------------------

// TransactionSearchResponse represents the PayPal transaction search response.
type TransactionSearchResponse struct {
	TransactionDetails []TransactionDetail `json:"transaction_details"`
	TotalItems         int                 `json:"total_items"`
	TotalPages         int                 `json:"total_pages"`
	Page               int                 `json:"page"`
}

// TransactionDetail holds individual transaction data from the search API.
type TransactionDetail struct {
	TransactionInfo TransactionInfo `json:"transaction_info"`
	PayerInfo       *PayerInfo      `json:"payer_info"`
}

// TransactionInfo contains the core transaction fields.
type TransactionInfo struct {
	TransactionID             string  `json:"transaction_id"`
	TransactionEventCode      string  `json:"transaction_event_code"`
	TransactionInitiationDate string  `json:"transaction_initiation_date"`
	TransactionUpdatedDate    string  `json:"transaction_updated_date"`
	TransactionAmount         *Money  `json:"transaction_amount"`
	FeeAmount                 *Money  `json:"fee_amount"`
	TransactionStatus         string  `json:"transaction_status"`
	TransactionSubject        *string `json:"transaction_subject"`
	TransactionNote           *string `json:"transaction_note"`
}

// PayerInfo holds payer details from transactions.
type PayerInfo struct {
	AccountID    *string    `json:"account_id"`
	EmailAddress *string    `json:"email_address"`
	PayerName    *PayerName `json:"payer_name"`
}

// PayerName holds given and surname.
type PayerName struct {
	GivenName *string `json:"given_name"`
	Surname   *string `json:"surname"`
}

// Money represents a PayPal monetary value.
type Money struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

// SearchTransactions searches for transactions in the given date range,
// handling PayPal's 31-day maximum window by batching requests.
func (c *PayPalClient) SearchTransactions(startDate, endDate string) ([]TransactionDetail, error) {
	start, err := time.Parse(time.RFC3339, startDate)
	if err != nil {
		return nil, fmt.Errorf("parse start date: %w", err)
	}
	end, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		return nil, fmt.Errorf("parse end date: %w", err)
	}

	var allTransactions []TransactionDetail
	windowStart := start

	for windowStart.Before(end) {
		windowEnd := windowStart.Add(31 * 24 * time.Hour)
		if windowEnd.After(end) {
			windowEnd = end
		}

		page := 1
		totalPages := 1

		for page <= totalPages {
			params := url.Values{}
			params.Set("start_date", windowStart.Format(time.RFC3339))
			params.Set("end_date", windowEnd.Format(time.RFC3339))
			params.Set("fields", "all")
			params.Set("page_size", "500")
			params.Set("page", fmt.Sprintf("%d", page))

			resp, err := c.doRequest("GET", "/v1/reporting/transactions?"+params.Encode(), nil)
			if err != nil {
				return allTransactions, fmt.Errorf("search transactions page %d: %w", page, err)
			}

			var result TransactionSearchResponse
			err = decodeAndClose(resp, &result)
			if err != nil {
				return allTransactions, err
			}

			allTransactions = append(allTransactions, result.TransactionDetails...)

			if result.TotalPages > 0 {
				totalPages = result.TotalPages
			}
			page++
		}

		windowStart = windowEnd
	}

	return allTransactions, nil
}

// --- Orders ------------------------------------------------------------------

// PayPalOrder represents a PayPal order resource.
type PayPalOrder struct {
	ID            string          `json:"id"`
	Status        string          `json:"status"`
	Intent        string          `json:"intent"`
	PurchaseUnits json.RawMessage `json:"purchase_units"`
	Payer         json.RawMessage `json:"payer"`
	CreateTime    string          `json:"create_time"`
	UpdateTime    string          `json:"update_time"`
}

// GetOrder retrieves a single order by ID.
func (c *PayPalClient) GetOrder(id string) (*PayPalOrder, error) {
	resp, err := c.doRequest("GET", "/v2/checkout/orders/"+id, nil)
	if err != nil {
		return nil, err
	}
	var order PayPalOrder
	if err := decodeAndClose(resp, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// --- Subscriptions -----------------------------------------------------------

// PayPalSubscription represents a PayPal subscription resource.
type PayPalSubscription struct {
	ID          string          `json:"id"`
	PlanID      string          `json:"plan_id"`
	Status      string          `json:"status"`
	Subscriber  json.RawMessage `json:"subscriber"`
	StartTime   string          `json:"start_time"`
	BillingInfo json.RawMessage `json:"billing_info"`
	CreateTime  string          `json:"create_time"`
	UpdateTime  string          `json:"update_time"`
}

// GetSubscription retrieves a single subscription by ID.
func (c *PayPalClient) GetSubscription(id string) (*PayPalSubscription, error) {
	resp, err := c.doRequest("GET", "/v1/billing/subscriptions/"+id, nil)
	if err != nil {
		return nil, err
	}
	var sub PayPalSubscription
	if err := decodeAndClose(resp, &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

// --- Subscription Plans ------------------------------------------------------

// PayPalSubscriptionPlan represents a PayPal billing plan.
type PayPalSubscriptionPlan struct {
	ID                 string          `json:"id"`
	ProductID          string          `json:"product_id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Status             string          `json:"status"`
	BillingCycles      json.RawMessage `json:"billing_cycles"`
	PaymentPreferences json.RawMessage `json:"payment_preferences"`
	CreateTime         string          `json:"create_time"`
	UpdateTime         string          `json:"update_time"`
}

// ListSubscriptionPlans retrieves all subscription plans with pagination.
func (c *PayPalClient) ListSubscriptionPlans() ([]PayPalSubscriptionPlan, error) {
	var plans []PayPalSubscriptionPlan
	page := 1

	for {
		params := url.Values{}
		params.Set("page_size", "20")
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("total_required", "true")

		resp, err := c.doRequest("GET", "/v1/billing/plans?"+params.Encode(), nil)
		if err != nil {
			return plans, err
		}

		var result struct {
			Plans      []PayPalSubscriptionPlan `json:"plans"`
			TotalPages int                      `json:"total_pages"`
		}
		if err := decodeAndClose(resp, &result); err != nil {
			return plans, err
		}

		plans = append(plans, result.Plans...)

		if result.TotalPages == 0 || page >= result.TotalPages || len(result.Plans) == 0 {
			break
		}
		page++
	}

	return plans, nil
}

// --- Products ----------------------------------------------------------------

// PayPalProduct represents a PayPal catalog product.
type PayPalProduct struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	ImageURL    string `json:"image_url"`
	HomeURL     string `json:"home_url"`
	CreateTime  string `json:"create_time"`
	UpdateTime  string `json:"update_time"`
}

// ListProducts retrieves all catalog products with pagination.
func (c *PayPalClient) ListProducts() ([]PayPalProduct, error) {
	var products []PayPalProduct
	page := 1

	for {
		params := url.Values{}
		params.Set("page_size", "20")
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("total_required", "true")

		resp, err := c.doRequest("GET", "/v1/catalogs/products?"+params.Encode(), nil)
		if err != nil {
			return products, err
		}

		var result struct {
			Products   []PayPalProduct `json:"products"`
			TotalPages int             `json:"total_pages"`
		}
		if err := decodeAndClose(resp, &result); err != nil {
			return products, err
		}

		products = append(products, result.Products...)

		if result.TotalPages == 0 || page >= result.TotalPages || len(result.Products) == 0 {
			break
		}
		page++
	}

	return products, nil
}

// --- Disputes ----------------------------------------------------------------

// PayPalDispute represents a PayPal dispute resource.
type PayPalDispute struct {
	DisputeID     string          `json:"dispute_id"`
	Reason        string          `json:"reason"`
	Status        string          `json:"status"`
	DisputeAmount *Money          `json:"dispute_amount"`
	Messages      json.RawMessage `json:"messages"`
	CreateTime    string          `json:"create_time"`
	UpdateTime    string          `json:"update_time"`
}

// ListDisputes retrieves all disputes with pagination.
func (c *PayPalClient) ListDisputes() ([]PayPalDispute, error) {
	var disputes []PayPalDispute

	params := url.Values{}
	params.Set("page_size", "50")

	resp, err := c.doRequest("GET", "/v1/customer/disputes?"+params.Encode(), nil)
	if err != nil {
		return disputes, err
	}

	var result struct {
		Items []PayPalDispute `json:"items"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if err := decodeAndClose(resp, &result); err != nil {
		return disputes, err
	}

	disputes = append(disputes, result.Items...)

	return disputes, nil
}

// --- Payouts -----------------------------------------------------------------

// PayPalPayoutBatch represents a PayPal payout batch resource.
type PayPalPayoutBatch struct {
	BatchHeader struct {
		PayoutBatchID     string `json:"payout_batch_id"`
		BatchStatus       string `json:"batch_status"`
		TimeCreated       string `json:"time_created"`
		SenderBatchHeader struct {
			SenderBatchID string `json:"sender_batch_id"`
		} `json:"sender_batch_header"`
		Amount *Money `json:"amount"`
		Fees   *Money `json:"fees"`
	} `json:"batch_header"`
}

// ListPayoutBatches retrieves payout batches. PayPal does not have a paginated list,
// so this returns whatever is available from the latest call.
func (c *PayPalClient) ListPayoutBatches() ([]PayPalPayoutBatch, error) {
	// PayPal's payout API does not have a list endpoint.
	// Payouts are typically tracked via webhooks or known batch IDs.
	return nil, nil
}

// --- Invoices ----------------------------------------------------------------

// PayPalInvoice represents a PayPal invoice resource.
type PayPalInvoice struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Detail     json.RawMessage `json:"detail"`
	Amount     *Money          `json:"amount"`
	DueAmount  *Money          `json:"due_amount"`
	Invoicer   json.RawMessage `json:"invoicer"`
	CreateTime string          `json:"create_time"`
	UpdateTime string          `json:"update_time"`
}

// ListInvoices retrieves all invoices with pagination.
func (c *PayPalClient) ListInvoices() ([]PayPalInvoice, error) {
	var invoices []PayPalInvoice
	page := 1

	for {
		params := url.Values{}
		params.Set("page_size", "100")
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("total_required", "true")

		resp, err := c.doRequest("GET", "/v2/invoicing/invoices?"+params.Encode(), nil)
		if err != nil {
			return invoices, err
		}

		var result struct {
			Items      []PayPalInvoice `json:"items"`
			TotalPages int             `json:"total_pages"`
		}
		if err := decodeAndClose(resp, &result); err != nil {
			return invoices, err
		}

		invoices = append(invoices, result.Items...)

		if result.TotalPages == 0 || page >= result.TotalPages || len(result.Items) == 0 {
			break
		}
		page++
	}

	return invoices, nil
}

// --- Helpers -----------------------------------------------------------------

// decodeAndClose reads the response body, checks for errors, and decodes JSON.
func decodeAndClose(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("paypal api error (status %d): %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(v)
}
