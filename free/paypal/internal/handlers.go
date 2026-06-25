package internal

import (

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterRoutes mounts all PayPal plugin endpoints on the given router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, cfg *Config) {
	// Webhooks
	r.Post("/webhooks/paypal", HandleWebhook(pool, cfg))

	// Sync operations
	r.Post("/v1/sync", handleSync(pool, cfg))
	r.Post("/v1/reconcile", handleReconcile(pool, cfg))

	// Transactions
	r.Get("/v1/transactions", handleListTransactions(pool))
	r.Get("/v1/transactions/{id}", handleGetTransaction(pool))

	// Orders
	r.Get("/v1/orders", handleListOrders(pool))
	r.Get("/v1/orders/{id}", handleGetOrder(pool))

	// Subscriptions
	r.Get("/v1/subscriptions", handleListSubscriptions(pool))
	r.Get("/v1/subscriptions/{id}", handleGetSubscription(pool))

	// Products
	r.Get("/v1/products", handleListProducts(pool))

	// Disputes
	r.Get("/v1/disputes", handleListDisputes(pool))

	// Invoices
	r.Get("/v1/invoices", handleListInvoices(pool))

	// Webhook events
	r.Get("/v1/webhook-events", handleListWebhookEvents(pool))

	// Stats
	r.Get("/v1/stats", handleStats(pool))
}

// --- Sync handlers -----------------------------------------------------------
