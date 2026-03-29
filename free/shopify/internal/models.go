package internal

import (
	"encoding/json"
	"time"
)

// Shop represents a row in np_shopify_shops.
type Shop struct {
	ID              string     `json:"id"`
	ShopifyID       int64      `json:"shopify_id"`
	Name            string     `json:"name"`
	Email           *string    `json:"email"`
	Domain          *string    `json:"domain"`
	MyshopifyDomain string     `json:"myshopify_domain"`
	Country         *string    `json:"country"`
	Currency        string     `json:"currency"`
	Timezone        *string    `json:"timezone"`
	PlanName        *string    `json:"plan_name"`
	PlanDisplayName *string    `json:"plan_display_name"`
	MoneyFormat     *string    `json:"money_format"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Product represents a row in np_shopify_products.
type Product struct {
	ID              string          `json:"id"`
	ShopifyID       int64           `json:"shopify_id"`
	Title           string          `json:"title"`
	BodyHTML        *string         `json:"body_html"`
	Vendor          *string         `json:"vendor"`
	ProductType     *string         `json:"product_type"`
	Handle          *string         `json:"handle"`
	Status          string          `json:"status"`
	Tags            *string         `json:"tags"`
	Images          json.RawMessage `json:"images"`
	Options         json.RawMessage `json:"options"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	PublishedAt     *time.Time      `json:"published_at"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// Variant represents a row in np_shopify_variants.
type Variant struct {
	ID                string     `json:"id"`
	ShopifyID         int64      `json:"shopify_id"`
	ProductID         string     `json:"product_id"`
	Title             *string    `json:"title"`
	Price             *string    `json:"price"`
	CompareAtPrice    *string    `json:"compare_at_price"`
	SKU               *string    `json:"sku"`
	Barcode           *string    `json:"barcode"`
	Position          int        `json:"position"`
	InventoryQuantity int        `json:"inventory_quantity"`
	InventoryItemID   *int64     `json:"inventory_item_id"`
	Weight            *float64   `json:"weight"`
	WeightUnit        *string    `json:"weight_unit"`
	Option1           *string    `json:"option1"`
	Option2           *string    `json:"option2"`
	Option3           *string    `json:"option3"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	SourceAccountID   string     `json:"source_account_id"`
	SyncedAt          *time.Time `json:"synced_at"`
}

// Collection represents a row in np_shopify_collections.
type Collection struct {
	ID              string          `json:"id"`
	ShopifyID       int64           `json:"shopify_id"`
	Title           string          `json:"title"`
	BodyHTML        *string         `json:"body_html"`
	Handle          *string         `json:"handle"`
	SortOrder       *string         `json:"sort_order"`
	CollectionType  *string         `json:"collection_type"`
	Image           json.RawMessage `json:"image"`
	PublishedAt     *time.Time      `json:"published_at"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// Customer represents a row in np_shopify_customers.
type Customer struct {
	ID               string          `json:"id"`
	ShopifyID        int64           `json:"shopify_id"`
	Email            *string         `json:"email"`
	FirstName        *string         `json:"first_name"`
	LastName         *string         `json:"last_name"`
	Phone            *string         `json:"phone"`
	OrdersCount      int             `json:"orders_count"`
	TotalSpent       *string         `json:"total_spent"`
	Currency         *string         `json:"currency"`
	Tags             *string         `json:"tags"`
	Addresses        json.RawMessage `json:"addresses"`
	DefaultAddress   json.RawMessage `json:"default_address"`
	AcceptsMarketing bool            `json:"accepts_marketing"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	SourceAccountID  string          `json:"source_account_id"`
	SyncedAt         *time.Time      `json:"synced_at"`
}

// Order represents a row in np_shopify_orders.
type Order struct {
	ID                string          `json:"id"`
	ShopifyID         int64           `json:"shopify_id"`
	Name              string          `json:"name"`
	Email             *string         `json:"email"`
	TotalPrice        *string         `json:"total_price"`
	SubtotalPrice     *string         `json:"subtotal_price"`
	TotalTax          *string         `json:"total_tax"`
	TotalDiscounts    *string         `json:"total_discounts"`
	Currency          string          `json:"currency"`
	FinancialStatus   *string         `json:"financial_status"`
	FulfillmentStatus *string         `json:"fulfillment_status"`
	CustomerID        *int64          `json:"customer_id"`
	LineItems         json.RawMessage `json:"line_items"`
	ShippingAddress   json.RawMessage `json:"shipping_address"`
	BillingAddress    json.RawMessage `json:"billing_address"`
	Note              *string         `json:"note"`
	Tags              *string         `json:"tags"`
	Gateway           *string         `json:"gateway"`
	Confirmed         bool            `json:"confirmed"`
	CancelledAt       *time.Time      `json:"cancelled_at"`
	CancelReason      *string         `json:"cancel_reason"`
	ClosedAt          *time.Time      `json:"closed_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ProcessedAt       *time.Time      `json:"processed_at"`
	SourceAccountID   string          `json:"source_account_id"`
	SyncedAt          *time.Time      `json:"synced_at"`
}

// OrderItem represents a row in np_shopify_order_items.
type OrderItem struct {
	ID                string     `json:"id"`
	ShopifyID         int64      `json:"shopify_id"`
	OrderID           string     `json:"order_id"`
	ProductID         *int64     `json:"product_id"`
	VariantID         *int64     `json:"variant_id"`
	Title             string     `json:"title"`
	Quantity          int        `json:"quantity"`
	Price             *string    `json:"price"`
	SKU               *string    `json:"sku"`
	Vendor            *string    `json:"vendor"`
	FulfillmentStatus *string    `json:"fulfillment_status"`
	SourceAccountID   string     `json:"source_account_id"`
	SyncedAt          *time.Time `json:"synced_at"`
}

// InventoryLevel represents a row in np_shopify_inventory.
type InventoryLevel struct {
	ID              string     `json:"id"`
	InventoryItemID int64      `json:"inventory_item_id"`
	LocationID      int64      `json:"location_id"`
	Available       int        `json:"available"`
	UpdatedAt       time.Time  `json:"updated_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// WebhookEvent represents a row in np_shopify_webhook_events.
type WebhookEvent struct {
	ID              string          `json:"id"`
	ShopifyEventID  *string         `json:"shopify_event_id"`
	Topic           string          `json:"topic"`
	ShopDomain      *string         `json:"shop_domain"`
	Body            json.RawMessage `json:"body"`
	Processed       bool            `json:"processed"`
	CreatedAt       time.Time       `json:"created_at"`
	SourceAccountID string          `json:"source_account_id"`
}

// SyncResult holds the summary of a full sync operation.
type SyncResult struct {
	Products    int    `json:"products"`
	Variants    int    `json:"variants"`
	Collections int    `json:"collections"`
	Customers   int    `json:"customers"`
	Orders      int    `json:"orders"`
	OrderItems  int    `json:"order_items"`
	Inventory   int    `json:"inventory"`
	AccountID   string `json:"account_id"`
	Error       string `json:"error,omitempty"`
}

// SyncStats holds aggregate counts returned by the stats endpoint.
type SyncStats struct {
	Shops       int        `json:"shops"`
	Products    int        `json:"products"`
	Variants    int        `json:"variants"`
	Collections int        `json:"collections"`
	Customers   int        `json:"customers"`
	Orders      int        `json:"orders"`
	OrderItems  int        `json:"order_items"`
	Inventory   int        `json:"inventory"`
	Events      int        `json:"events"`
	LastSynced  *time.Time `json:"last_synced"`
}

// Shopify API response wrappers for JSON unmarshalling.

type shopifyShopResponse struct {
	Shop shopifyShop `json:"shop"`
}

type shopifyShop struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	Domain          string  `json:"domain"`
	MyshopifyDomain string  `json:"myshopify_domain"`
	Country         string  `json:"country"`
	Currency        string  `json:"currency"`
	Timezone        string  `json:"timezone"`
	PlanName        string  `json:"plan_name"`
	PlanDisplayName string  `json:"plan_display_name"`
	MoneyFormat     string  `json:"money_format"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type shopifyProductsResponse struct {
	Products []shopifyProduct `json:"products"`
}

type shopifyProductResponse struct {
	Product shopifyProduct `json:"product"`
}

type shopifyProduct struct {
	ID          int64              `json:"id"`
	Title       string             `json:"title"`
	BodyHTML    string             `json:"body_html"`
	Vendor      string             `json:"vendor"`
	ProductType string             `json:"product_type"`
	Handle      string             `json:"handle"`
	Status      string             `json:"status"`
	Tags        string             `json:"tags"`
	Images      json.RawMessage    `json:"images"`
	Options     json.RawMessage    `json:"options"`
	Variants    []shopifyVariant   `json:"variants"`
	PublishedAt *string            `json:"published_at"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

type shopifyVariant struct {
	ID                int64    `json:"id"`
	ProductID         int64    `json:"product_id"`
	Title             string   `json:"title"`
	Price             string   `json:"price"`
	CompareAtPrice    *string  `json:"compare_at_price"`
	SKU               string   `json:"sku"`
	Barcode           *string  `json:"barcode"`
	Position          int      `json:"position"`
	InventoryQuantity int      `json:"inventory_quantity"`
	InventoryItemID   int64    `json:"inventory_item_id"`
	Weight            float64  `json:"weight"`
	WeightUnit        string   `json:"weight_unit"`
	Option1           *string  `json:"option1"`
	Option2           *string  `json:"option2"`
	Option3           *string  `json:"option3"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type shopifyCollectionsResponse struct {
	CustomCollections []shopifyCollection `json:"custom_collections"`
	SmartCollections  []shopifyCollection `json:"smart_collections"`
}

type shopifyCollection struct {
	ID             int64           `json:"id"`
	Title          string          `json:"title"`
	BodyHTML       string          `json:"body_html"`
	Handle         string          `json:"handle"`
	SortOrder      string          `json:"sort_order"`
	CollectionType string          `json:"collection_type,omitempty"`
	Image          json.RawMessage `json:"image"`
	PublishedAt    *string         `json:"published_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type shopifyCustomersResponse struct {
	Customers []shopifyCustomer `json:"customers"`
}

type shopifyCustomer struct {
	ID               int64           `json:"id"`
	Email            string          `json:"email"`
	FirstName        string          `json:"first_name"`
	LastName         string          `json:"last_name"`
	Phone            *string         `json:"phone"`
	OrdersCount      int             `json:"orders_count"`
	TotalSpent       string          `json:"total_spent"`
	Currency         string          `json:"currency"`
	Tags             string          `json:"tags"`
	Addresses        json.RawMessage `json:"addresses"`
	DefaultAddress   json.RawMessage `json:"default_address"`
	AcceptsMarketing bool            `json:"accepts_marketing"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

type shopifyOrdersResponse struct {
	Orders []shopifyOrder `json:"orders"`
}

type shopifyOrderResponse struct {
	Order shopifyOrder `json:"order"`
}

type shopifyOrder struct {
	ID                int64              `json:"id"`
	Name              string             `json:"name"`
	Email             string             `json:"email"`
	TotalPrice        string             `json:"total_price"`
	SubtotalPrice     string             `json:"subtotal_price"`
	TotalTax          string             `json:"total_tax"`
	TotalDiscounts    string             `json:"total_discounts"`
	Currency          string             `json:"currency"`
	FinancialStatus   string             `json:"financial_status"`
	FulfillmentStatus *string            `json:"fulfillment_status"`
	CustomerID        *int64             `json:"customer_id,omitempty"`
	Customer          *shopifyCustomer   `json:"customer,omitempty"`
	LineItems         []shopifyLineItem  `json:"line_items"`
	ShippingAddress   json.RawMessage    `json:"shipping_address"`
	BillingAddress    json.RawMessage    `json:"billing_address"`
	Note              *string            `json:"note"`
	Tags              string             `json:"tags"`
	Gateway           string             `json:"gateway"`
	Confirmed         bool               `json:"confirmed"`
	CancelledAt       *string            `json:"cancelled_at"`
	CancelReason      *string            `json:"cancel_reason"`
	ClosedAt          *string            `json:"closed_at"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
	ProcessedAt       *string            `json:"processed_at"`
}

type shopifyLineItem struct {
	ID                int64   `json:"id"`
	ProductID         *int64  `json:"product_id"`
	VariantID         *int64  `json:"variant_id"`
	Title             string  `json:"title"`
	Quantity          int     `json:"quantity"`
	Price             string  `json:"price"`
	SKU               string  `json:"sku"`
	Vendor            string  `json:"vendor"`
	FulfillmentStatus *string `json:"fulfillment_status"`
}

type shopifyLocationsResponse struct {
	Locations []shopifyLocation `json:"locations"`
}

type shopifyLocation struct {
	ID int64 `json:"id"`
}

type shopifyInventoryLevelsResponse struct {
	InventoryLevels []shopifyInventoryLevel `json:"inventory_levels"`
}

type shopifyInventoryLevel struct {
	InventoryItemID int64  `json:"inventory_item_id"`
	LocationID      int64  `json:"location_id"`
	Available       int    `json:"available"`
	UpdatedAt       string `json:"updated_at"`
}

// ListParams holds optional parameters for paginated Shopify API requests.
type ListParams struct {
	Limit     int
	PageInfo  string
	SinceID   int64
	Status    string
	CreatedAt string
}
