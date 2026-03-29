package internal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
)

// WebhookProcessor handles incoming Shopify webhook events.
// It stores each event, dispatches to the correct handler, and marks it processed.
type WebhookProcessor struct {
	DB     *DB
	Client *ShopifyAPIClient
}

// NewWebhookProcessor creates a webhook processor.
func NewWebhookProcessor(db *DB, client *ShopifyAPIClient) *WebhookProcessor {
	return &WebhookProcessor{DB: db, Client: client}
}

// VerifyHMAC verifies a Shopify webhook HMAC-SHA256 signature.
// Shopify sends the signature as a base64-encoded HMAC-SHA256 hash
// in the X-Shopify-Hmac-Sha256 header.
func VerifyHMAC(body []byte, secret, headerValue string) bool {
	if secret == "" || headerValue == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(headerValue)) == 1
}

// ProcessEvent stores and dispatches a webhook event.
func (wp *WebhookProcessor) ProcessEvent(ctx context.Context, shopifyEventID, topic, shopDomain string, rawBody []byte) error {
	if shopifyEventID == "" {
		shopifyEventID = uuid.New().String()
	}

	// Store the raw event
	if err := wp.DB.InsertWebhookEvent(ctx, shopifyEventID, topic, shopDomain, rawBody); err != nil {
		return fmt.Errorf("failed to store webhook event: %w", err)
	}

	log.Printf("[shopify:webhook] Event received: topic=%s shop=%s", topic, shopDomain)

	// Parse payload for dispatching
	var payload map[string]interface{}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		payload = map[string]interface{}{}
	}

	// Dispatch to handler
	if err := wp.dispatch(ctx, topic, payload); err != nil {
		log.Printf("[shopify:webhook] Handler error: topic=%s err=%v", topic, err)
		return err
	}

	log.Printf("[shopify:webhook] Event processed: topic=%s", topic)
	return nil
}

func (wp *WebhookProcessor) dispatch(ctx context.Context, topic string, payload map[string]interface{}) error {
	switch topic {
	// Orders
	case "orders/create", "orders/updated", "orders/paid", "orders/cancelled":
		return wp.handleOrderSync(ctx, payload)
	case "orders/delete":
		return wp.handleOrderDelete(ctx, payload)

	// Products
	case "products/create", "products/update":
		return wp.handleProductSync(ctx, payload)
	case "products/delete":
		return wp.handleProductDelete(ctx, payload)

	// Customers
	case "customers/create", "customers/update":
		return wp.handleCustomerSync(ctx, payload)
	case "customers/delete":
		return wp.handleCustomerDelete(ctx, payload)

	// Inventory
	case "inventory_levels/update":
		return wp.handleInventoryUpdate(ctx, payload)
	case "inventory_levels/connect":
		return wp.handleInventoryConnect(ctx, payload)
	case "inventory_levels/disconnect":
		return wp.handleInventoryDisconnect(ctx, payload)

	// Fulfillments (re-sync the parent order)
	case "fulfillments/create", "fulfillments/update":
		orderID := jsonInt64(payload, "order_id")
		if orderID != 0 {
			return wp.handleOrderSync(ctx, map[string]interface{}{"id": float64(orderID)})
		}
		return nil

	// Refunds (re-sync the parent order)
	case "refunds/create":
		orderID := jsonInt64(payload, "order_id")
		if orderID != 0 {
			return wp.handleOrderSync(ctx, map[string]interface{}{"id": float64(orderID)})
		}
		return nil

	// Collections
	case "collections/create", "collections/update":
		log.Printf("[shopify:webhook] Collection event: %s (informational)", topic)
		return nil
	case "collections/delete":
		return wp.handleCollectionDelete(ctx, payload)

	default:
		log.Printf("[shopify:webhook] No handler for topic: %s", topic)
		return nil
	}
}

// -------------------------------------------------------------------------
// Handlers
// -------------------------------------------------------------------------

func (wp *WebhookProcessor) handleOrderSync(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return fmt.Errorf("order webhook missing id")
	}
	order, items, err := wp.Client.GetOrder(ctx, shopifyID)
	if err != nil {
		return fmt.Errorf("failed to fetch order %d: %w", shopifyID, err)
	}
	if order == nil {
		return nil
	}
	orderUUID, err := wp.DB.UpsertOrder(ctx, order)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].OrderID = orderUUID
		if err := wp.DB.UpsertOrderItem(ctx, &items[i]); err != nil {
			log.Printf("[shopify:webhook] Failed to upsert order item: %v", err)
		}
	}
	return nil
}

func (wp *WebhookProcessor) handleOrderDelete(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return nil
	}
	return wp.DB.DeleteOrderByShopifyID(ctx, shopifyID)
}

func (wp *WebhookProcessor) handleProductSync(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return fmt.Errorf("product webhook missing id")
	}
	product, variants, err := wp.Client.GetProduct(ctx, shopifyID)
	if err != nil {
		return fmt.Errorf("failed to fetch product %d: %w", shopifyID, err)
	}
	if product == nil {
		return nil
	}
	productUUID, err := wp.DB.UpsertProduct(ctx, product)
	if err != nil {
		return err
	}
	for i := range variants {
		variants[i].ProductID = productUUID
		if err := wp.DB.UpsertVariant(ctx, &variants[i]); err != nil {
			log.Printf("[shopify:webhook] Failed to upsert variant: %v", err)
		}
	}
	return nil
}

func (wp *WebhookProcessor) handleProductDelete(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return nil
	}
	return wp.DB.DeleteProductByShopifyID(ctx, shopifyID)
}

func (wp *WebhookProcessor) handleCustomerSync(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return fmt.Errorf("customer webhook missing id")
	}
	customer, err := wp.Client.GetCustomer(ctx, shopifyID)
	if err != nil {
		return fmt.Errorf("failed to fetch customer %d: %w", shopifyID, err)
	}
	if customer == nil {
		return nil
	}
	return wp.DB.UpsertCustomer(ctx, customer)
}

func (wp *WebhookProcessor) handleCustomerDelete(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return nil
	}
	return wp.DB.DeleteCustomerByShopifyID(ctx, shopifyID)
}

func (wp *WebhookProcessor) handleInventoryUpdate(ctx context.Context, payload map[string]interface{}) error {
	itemID := jsonInt64(payload, "inventory_item_id")
	locID := jsonInt64(payload, "location_id")
	available := jsonInt(payload, "available")
	if itemID == 0 || locID == 0 {
		return fmt.Errorf("inventory update missing inventory_item_id or location_id")
	}
	return wp.DB.UpsertInventoryLevel(ctx, itemID, locID, available)
}

func (wp *WebhookProcessor) handleInventoryConnect(ctx context.Context, payload map[string]interface{}) error {
	itemID := jsonInt64(payload, "inventory_item_id")
	locID := jsonInt64(payload, "location_id")
	if itemID == 0 || locID == 0 {
		return nil
	}
	return wp.DB.UpsertInventoryLevel(ctx, itemID, locID, 0)
}

func (wp *WebhookProcessor) handleInventoryDisconnect(ctx context.Context, payload map[string]interface{}) error {
	itemID := jsonInt64(payload, "inventory_item_id")
	locID := jsonInt64(payload, "location_id")
	if itemID == 0 || locID == 0 {
		return nil
	}
	return wp.DB.DeleteInventoryLevel(ctx, itemID, locID)
}

func (wp *WebhookProcessor) handleCollectionDelete(ctx context.Context, payload map[string]interface{}) error {
	shopifyID := jsonInt64(payload, "id")
	if shopifyID == 0 {
		return nil
	}
	return wp.DB.DeleteCollectionByShopifyID(ctx, shopifyID)
}

// -------------------------------------------------------------------------
// JSON helpers
// -------------------------------------------------------------------------

func jsonInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func jsonInt(m map[string]interface{}, key string) int {
	return int(jsonInt64(m, key))
}
