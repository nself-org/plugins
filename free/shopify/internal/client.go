package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ShopifyAPIClient communicates with the Shopify Admin REST API.
type ShopifyAPIClient struct {
	accessToken string
	shopDomain  string
	apiVersion  string
	httpClient  *http.Client
}

// NewShopifyAPIClient creates a new Shopify API client.
func NewShopifyAPIClient(shopDomain, accessToken, apiVersion string) *ShopifyAPIClient {
	return &ShopifyAPIClient{
		accessToken: accessToken,
		shopDomain:  shopDomain,
		apiVersion:  apiVersion,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// baseURL returns the Shopify Admin API base URL.
func (c *ShopifyAPIClient) baseURL() string {
	return fmt.Sprintf("https://%s/admin/api/%s", c.shopDomain, c.apiVersion)
}

// doRequest executes an authenticated request with rate limiting.
func (c *ShopifyAPIClient) doRequest(method, endpoint string, query url.Values) ([]byte, http.Header, error) {
	reqURL := c.baseURL() + endpoint
	if query != nil && len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, reqURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Shopify-Access-Token", c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	// Shopify allows 2 requests per second. Add 500ms delay between calls.
	time.Sleep(500 * time.Millisecond)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	// Handle rate limiting with Retry-After header.
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		waitSec := 2.0
		if retryAfter != "" {
			if parsed, parseErr := strconv.ParseFloat(retryAfter, 64); parseErr == nil {
				waitSec = parsed
			}
		}
		time.Sleep(time.Duration(waitSec*1000) * time.Millisecond)
		// Retry once after waiting.
		return c.doRequest(method, endpoint, query)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("shopify API error %d: %s", resp.StatusCode, string(body))
	}

	return body, resp.Header, nil
}

// parseLinkPageInfo extracts the page_info cursor from the Link header for pagination.
func parseLinkPageInfo(header http.Header) string {
	link := header.Get("Link")
	if link == "" {
		return ""
	}
	// Look for rel="next" link.
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		// Extract URL between < and >.
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		linkURL := part[start+1 : end]
		parsed, err := url.Parse(linkURL)
		if err != nil {
			continue
		}
		return parsed.Query().Get("page_info")
	}
	return ""
}

// fetchShop fetches the shop information from the Shopify API.
func (c *ShopifyAPIClient) fetchShop() (*shopifyShop, error) {
	body, _, err := c.doRequest("GET", "/shop.json", nil)
	if err != nil {
		return nil, err
	}
	var resp shopifyShopResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing shop response: %w", err)
	}
	return &resp.Shop, nil
}
