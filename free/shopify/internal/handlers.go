package internal

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterRoutes registers all Shopify plugin HTTP routes on the given chi router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, cfg *Config) {
	db := NewDB(pool, "primary")
	client := NewShopifyAPIClient(cfg.ShopDomain, cfg.AccessToken, cfg.APIVersion)
	webhookProc := NewWebhookProcessor(db, client)

	// Health checks
	r.Get("/health", handleHealth)
	r.Get("/ready", handleReady(db))
	r.Get("/live", handleLive(db))

	// Status / stats
	r.Get("/status", handleStatus(db))
	r.Get("/api/stats", handleStats(db))

	// Webhooks
	r.Post("/webhooks/shopify", handleWebhook(webhookProc, cfg.WebhookSecret))

	// Sync
	r.Post("/sync", handleSync(db, client))
	r.Post("/api/sync", handleSync(db, client))

	// API: Shops
	r.Get("/api/shops", handleListShops(db))

	// API: Products
	r.Get("/api/products", handleListProducts(db))
	r.Get("/api/products/{id}", handleGetProduct(db))

	// API: Variants
	r.Get("/api/variants", handleListVariants(db))

	// API: Collections
	r.Get("/api/collections", handleListCollections(db))

	// API: Customers
	r.Get("/api/customers", handleListCustomers(db))
	r.Get("/api/customers/{id}", handleGetCustomer(db))

	// API: Orders
	r.Get("/api/orders", handleListOrders(db))
	r.Get("/api/orders/{id}", handleGetOrder(db))

	// API: Inventory
	r.Get("/api/inventory", handleListInventory(db))

	// API: Webhook Events
	r.Get("/api/events", handleListEvents(db))
}

