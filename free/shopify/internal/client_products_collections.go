package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ListProducts fetches products with cursor-based pagination.
func (c *ShopifyAPIClient) ListProducts(params ListParams) ([]shopifyProduct, string, error) {
	q := url.Values{}
	limit := params.Limit
	if limit <= 0 {
		limit = 250
	}
	q.Set("limit", strconv.Itoa(limit))

	if params.PageInfo != "" {
		q.Set("page_info", params.PageInfo)
	} else {
		if params.Status != "" {
			q.Set("status", params.Status)
		}
	}

	body, headers, err := c.doRequest("GET", "/products.json", q)
	if err != nil {
		return nil, "", err
	}

	var resp shopifyProductsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parsing products response: %w", err)
	}

	nextPageInfo := parseLinkPageInfo(headers)
	return resp.Products, nextPageInfo, nil
}

// fetchProduct fetches a single product by Shopify ID from the API.
func (c *ShopifyAPIClient) fetchProduct(id int64) (*shopifyProduct, error) {
	body, _, err := c.doRequest("GET", fmt.Sprintf("/products/%d.json", id), nil)
	if err != nil {
		return nil, err
	}
	var resp shopifyProductResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing product response: %w", err)
	}
	return &resp.Product, nil
}

// ListCollections fetches both custom and smart collections.
func (c *ShopifyAPIClient) ListCollections() ([]shopifyCollection, error) {
	var all []shopifyCollection

	// Fetch custom collections.
	q := url.Values{}
	q.Set("limit", "250")
	body, _, err := c.doRequest("GET", "/custom_collections.json", q)
	if err != nil {
		return nil, err
	}
	var customResp struct {
		CustomCollections []shopifyCollection `json:"custom_collections"`
	}
	if err := json.Unmarshal(body, &customResp); err != nil {
		return nil, fmt.Errorf("parsing custom collections response: %w", err)
	}
	for i := range customResp.CustomCollections {
		customResp.CustomCollections[i].CollectionType = "custom"
	}
	all = append(all, customResp.CustomCollections...)

	// Fetch smart collections.
	body, _, err = c.doRequest("GET", "/smart_collections.json", q)
	if err != nil {
		return nil, err
	}
	var smartResp struct {
		SmartCollections []shopifyCollection `json:"smart_collections"`
	}
	if err := json.Unmarshal(body, &smartResp); err != nil {
		return nil, fmt.Errorf("parsing smart collections response: %w", err)
	}
	for i := range smartResp.SmartCollections {
		smartResp.SmartCollections[i].CollectionType = "smart"
	}
	all = append(all, smartResp.SmartCollections...)

	return all, nil
}

// ListCustomers fetches customers with cursor-based pagination.
func (c *ShopifyAPIClient) ListCustomers(params ListParams) ([]shopifyCustomer, string, error) {
	q := url.Values{}
	limit := params.Limit
	if limit <= 0 {
		limit = 250
	}
	q.Set("limit", strconv.Itoa(limit))

	if params.PageInfo != "" {
		q.Set("page_info", params.PageInfo)
	}

	body, headers, err := c.doRequest("GET", "/customers.json", q)
	if err != nil {
		return nil, "", err
	}

	var resp shopifyCustomersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parsing customers response: %w", err)
	}

	nextPageInfo := parseLinkPageInfo(headers)
	return resp.Customers, nextPageInfo, nil
}

// ListOrders fetches orders with cursor-based pagination.
