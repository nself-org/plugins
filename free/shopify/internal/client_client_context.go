package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// -------------------------------------------------------------------------
// High-level context-aware methods used by handlers and webhooks.
// These fetch from the Shopify API and convert to DB model types.
// -------------------------------------------------------------------------

// GetShop fetches the shop info and returns the raw Shopify shop struct.
func (c *ShopifyAPIClient) GetShop(_ context.Context) (*shopifyShop, error) {
	return c.fetchShop()
}

// GetProduct fetches a single product by Shopify ID and converts to DB model types.
func (c *ShopifyAPIClient) GetProduct(_ context.Context, id int64) (*Product, []Variant, error) {
	sp, err := c.fetchProduct(id)
	if err != nil {
		return nil, nil, err
	}
	if sp == nil {
		return nil, nil, nil
	}
	product := convertProduct(sp)
	var variants []Variant
	for _, sv := range sp.Variants {
		variants = append(variants, convertVariant(&sv))
	}
	return &product, variants, nil
}

// GetOrder fetches a single order by Shopify ID and converts to DB model types.
func (c *ShopifyAPIClient) GetOrder(_ context.Context, id int64) (*Order, []OrderItem, error) {
	so, err := c.fetchOrder(id)
	if err != nil {
		return nil, nil, err
	}
	if so == nil {
		return nil, nil, nil
	}
	order := convertOrder(so)
	var items []OrderItem
	for _, li := range so.LineItems {
		items = append(items, convertLineItem(&li))
	}
	return &order, items, nil
}

// GetCustomer fetches a single customer by Shopify ID and converts to DB model type.
func (c *ShopifyAPIClient) GetCustomer(_ context.Context, id int64) (*Customer, error) {
	q := url.Values{}
	body, _, err := c.doRequest("GET", fmt.Sprintf("/customers/%d.json", id), q)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Customer shopifyCustomer `json:"customer"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing customer response: %w", err)
	}
	customer := convertCustomer(&resp.Customer)
	return &customer, nil
}

// ListAllProducts fetches all products (paginated) and converts to DB model types.
func (c *ShopifyAPIClient) ListAllProducts(_ context.Context) ([]Product, []Variant, error) {
	var allProducts []Product
	var allVariants []Variant
	params := ListParams{Limit: 250, Status: "any"}

	for {
		sProducts, nextPage, err := c.ListProducts(params)
		if err != nil {
			return nil, nil, err
		}
		for _, sp := range sProducts {
			p := convertProduct(&sp)
			allProducts = append(allProducts, p)
			for _, sv := range sp.Variants {
				allVariants = append(allVariants, convertVariant(&sv))
			}
		}
		if nextPage == "" {
			break
		}
		params.PageInfo = nextPage
	}

	return allProducts, allVariants, nil
}

// ListAllCollections fetches all collections and converts to DB model types.
func (c *ShopifyAPIClient) ListAllCollections(_ context.Context) ([]Collection, error) {
	sCollections, err := c.ListCollections()
	if err != nil {
		return nil, err
	}
	var results []Collection
	for _, sc := range sCollections {
		results = append(results, convertCollection(&sc))
	}
	return results, nil
}

// ListAllCustomers fetches all customers (paginated) and converts to DB model types.
func (c *ShopifyAPIClient) ListAllCustomers(_ context.Context) ([]Customer, error) {
	var all []Customer
	params := ListParams{Limit: 250}

	for {
		sCustomers, nextPage, err := c.ListCustomers(params)
		if err != nil {
			return nil, err
		}
		for _, sc := range sCustomers {
			all = append(all, convertCustomer(&sc))
		}
		if nextPage == "" {
			break
		}
		params.PageInfo = nextPage
	}

	return all, nil
}

// ListAllOrders fetches all orders (paginated) and converts to DB model types.
func (c *ShopifyAPIClient) ListAllOrders(_ context.Context) ([]Order, []OrderItem, error) {
	var allOrders []Order
	var allItems []OrderItem
	params := ListParams{Limit: 250}

	for {
		sOrders, nextPage, err := c.ListOrders(params)
		if err != nil {
			return nil, nil, err
		}
		for _, so := range sOrders {
			allOrders = append(allOrders, convertOrder(&so))
			for _, li := range so.LineItems {
				allItems = append(allItems, convertLineItem(&li))
			}
		}
		if nextPage == "" {
			break
		}
		params.PageInfo = nextPage
	}

	return allOrders, allItems, nil
}

// ListInventoryLevels fetches all inventory levels across all locations.
func (c *ShopifyAPIClient) ListInventoryLevels(_ context.Context) ([]shopifyInventoryLevel, error) {
	locations, err := c.ListLocations()
	if err != nil {
		return nil, err
	}
	var all []shopifyInventoryLevel
	for _, loc := range locations {
		levels, err := c.fetchInventoryLevels(loc.ID)
		if err != nil {
			return nil, err
		}
		all = append(all, levels...)
	}
	return all, nil
}

