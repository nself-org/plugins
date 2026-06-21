package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
)

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
