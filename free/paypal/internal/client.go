package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
