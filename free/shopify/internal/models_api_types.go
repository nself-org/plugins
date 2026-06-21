package internal

import (
	"encoding/json"
)

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
