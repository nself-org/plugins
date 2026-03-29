package internal

import (
	"context"
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

// -------------------------------------------------------------------------
// Conversion helpers: Shopify API types -> DB model types
// -------------------------------------------------------------------------

func convertProduct(sp *shopifyProduct) Product {
	p := Product{
		ShopifyID:   sp.ID,
		Title:       sp.Title,
		BodyHTML:     strPtr(sp.BodyHTML),
		Vendor:      strPtr(sp.Vendor),
		ProductType: strPtr(sp.ProductType),
		Handle:      strPtr(sp.Handle),
		Status:      sp.Status,
		Tags:        strPtr(sp.Tags),
		Images:      sp.Images,
		Options:     sp.Options,
	}
	if sp.PublishedAt != nil {
		if t, err := time.Parse(time.RFC3339, *sp.PublishedAt); err == nil {
			p.PublishedAt = &t
		}
	}
	return p
}

func convertVariant(sv *shopifyVariant) Variant {
	v := Variant{
		ShopifyID:         sv.ID,
		Title:             strPtr(sv.Title),
		Price:             strPtr(sv.Price),
		CompareAtPrice:    sv.CompareAtPrice,
		SKU:               strPtr(sv.SKU),
		Barcode:           sv.Barcode,
		Position:          sv.Position,
		InventoryQuantity: sv.InventoryQuantity,
		Weight:            &sv.Weight,
		WeightUnit:        strPtr(sv.WeightUnit),
		Option1:           sv.Option1,
		Option2:           sv.Option2,
		Option3:           sv.Option3,
	}
	if sv.InventoryItemID != 0 {
		v.InventoryItemID = &sv.InventoryItemID
	}
	return v
}

func convertCollection(sc *shopifyCollection) Collection {
	c := Collection{
		ShopifyID:      sc.ID,
		Title:          sc.Title,
		BodyHTML:        strPtr(sc.BodyHTML),
		Handle:         strPtr(sc.Handle),
		SortOrder:      strPtr(sc.SortOrder),
		CollectionType: strPtr(sc.CollectionType),
		Image:          sc.Image,
	}
	if sc.PublishedAt != nil {
		if t, err := time.Parse(time.RFC3339, *sc.PublishedAt); err == nil {
			c.PublishedAt = &t
		}
	}
	return c
}

func convertCustomer(sc *shopifyCustomer) Customer {
	return Customer{
		ShopifyID:        sc.ID,
		Email:            strPtr(sc.Email),
		FirstName:        strPtr(sc.FirstName),
		LastName:         strPtr(sc.LastName),
		Phone:            sc.Phone,
		OrdersCount:      sc.OrdersCount,
		TotalSpent:       strPtr(sc.TotalSpent),
		Currency:         strPtr(sc.Currency),
		Tags:             strPtr(sc.Tags),
		Addresses:        sc.Addresses,
		DefaultAddress:   sc.DefaultAddress,
		AcceptsMarketing: sc.AcceptsMarketing,
	}
}

func convertOrder(so *shopifyOrder) Order {
	o := Order{
		ShopifyID:         so.ID,
		Name:              so.Name,
		Email:             strPtr(so.Email),
		TotalPrice:        strPtr(so.TotalPrice),
		SubtotalPrice:     strPtr(so.SubtotalPrice),
		TotalTax:          strPtr(so.TotalTax),
		TotalDiscounts:    strPtr(so.TotalDiscounts),
		Currency:          so.Currency,
		FinancialStatus:   strPtr(so.FinancialStatus),
		FulfillmentStatus: so.FulfillmentStatus,
		CustomerID:        so.CustomerID,
		ShippingAddress:   so.ShippingAddress,
		BillingAddress:    so.BillingAddress,
		Note:              so.Note,
		Tags:              strPtr(so.Tags),
		Gateway:           strPtr(so.Gateway),
		Confirmed:         so.Confirmed,
		CancelReason:      so.CancelReason,
	}
	if so.LineItems != nil {
		lineItemsJSON, _ := json.Marshal(so.LineItems)
		o.LineItems = lineItemsJSON
	}
	if so.CancelledAt != nil {
		if t, err := time.Parse(time.RFC3339, *so.CancelledAt); err == nil {
			o.CancelledAt = &t
		}
	}
	if so.ClosedAt != nil {
		if t, err := time.Parse(time.RFC3339, *so.ClosedAt); err == nil {
			o.ClosedAt = &t
		}
	}
	if so.ProcessedAt != nil {
		if t, err := time.Parse(time.RFC3339, *so.ProcessedAt); err == nil {
			o.ProcessedAt = &t
		}
	}
	return o
}

func convertLineItem(li *shopifyLineItem) OrderItem {
	return OrderItem{
		ShopifyID:         li.ID,
		ProductID:         li.ProductID,
		VariantID:         li.VariantID,
		Title:             li.Title,
		Quantity:          li.Quantity,
		Price:             strPtr(li.Price),
		SKU:               strPtr(li.SKU),
		Vendor:            strPtr(li.Vendor),
		FulfillmentStatus: li.FulfillmentStatus,
	}
}

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
