package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

func (c *ShopifyAPIClient) ListOrders(params ListParams) ([]shopifyOrder, string, error) {
	q := url.Values{}
	limit := params.Limit
	if limit <= 0 {
		limit = 250
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("status", "any")

	if params.PageInfo != "" {
		q.Set("page_info", params.PageInfo)
	}

	body, headers, err := c.doRequest("GET", "/orders.json", q)
	if err != nil {
		return nil, "", err
	}

	var resp shopifyOrdersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parsing orders response: %w", err)
	}

	nextPageInfo := parseLinkPageInfo(headers)
	return resp.Orders, nextPageInfo, nil
}

// fetchOrder fetches a single order by Shopify ID from the API.
func (c *ShopifyAPIClient) fetchOrder(id int64) (*shopifyOrder, error) {
	body, _, err := c.doRequest("GET", fmt.Sprintf("/orders/%d.json", id), nil)
	if err != nil {
		return nil, err
	}
	var resp shopifyOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing order response: %w", err)
	}
	return &resp.Order, nil
}

// ListLocations fetches all locations for the shop.
func (c *ShopifyAPIClient) ListLocations() ([]shopifyLocation, error) {
	body, _, err := c.doRequest("GET", "/locations.json", nil)
	if err != nil {
		return nil, err
	}
	var resp shopifyLocationsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing locations response: %w", err)
	}
	return resp.Locations, nil
}

// fetchInventoryLevels fetches inventory levels for a given location from the API.
func (c *ShopifyAPIClient) fetchInventoryLevels(locationID int64) ([]shopifyInventoryLevel, error) {
	q := url.Values{}
	q.Set("location_ids", strconv.FormatInt(locationID, 10))
	q.Set("limit", "250")

	body, _, err := c.doRequest("GET", "/inventory_levels.json", q)
	if err != nil {
		return nil, err
	}

	var resp shopifyInventoryLevelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing inventory levels response: %w", err)
	}
	return resp.InventoryLevels, nil
}
