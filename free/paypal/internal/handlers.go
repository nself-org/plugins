package internal

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
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

func handleSync(pool *pgxpool.Pool, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		result := SyncAll(ctx, pool, cfg)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

func handleReconcile(pool *pgxpool.Pool, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		lookback := 7
		if v := r.URL.Query().Get("days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				lookback = n
			}
		}

		result := Reconcile(ctx, pool, cfg, lookback)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

// --- Transaction handlers ----------------------------------------------------

func handleListTransactions(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		transactions, err := ListTransactions(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if transactions == nil {
			transactions = []Transaction{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"transactions": transactions,
			"limit":        limit,
			"offset":       offset,
		})
	}
}

func handleGetTransaction(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		transaction, err := GetTransaction(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, transaction)
	}
}

// --- Order handlers ----------------------------------------------------------

func handleListOrders(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		orders, err := ListOrders(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if orders == nil {
			orders = []Order{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"orders": orders,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleGetOrder(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		order, err := GetOrder(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, order)
	}
}

// --- Subscription handlers ---------------------------------------------------

func handleListSubscriptions(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		subscriptions, err := ListSubscriptions(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if subscriptions == nil {
			subscriptions = []Subscription{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"subscriptions": subscriptions,
			"limit":         limit,
			"offset":        offset,
		})
	}
}

func handleGetSubscription(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		subscription, err := GetSubscription(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, subscription)
	}
}

// --- Products handler --------------------------------------------------------

func handleListProducts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		rows, err := pool.Query(ctx, `
			SELECT id, paypal_id, name, description, type, category, image_url, home_url, created_at, updated_at, source_account_id, synced_at
			FROM np_paypal_products
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		defer rows.Close()

		var products []Product
		for rows.Next() {
			var p Product
			if err := rows.Scan(&p.ID, &p.PayPalID, &p.Name, &p.Description, &p.Type, &p.Category,
				&p.ImageURL, &p.HomeURL, &p.CreatedAt, &p.UpdatedAt, &p.SourceAccountID, &p.SyncedAt); err != nil {
				sdk.Error(w, http.StatusInternalServerError, err)
				return
			}
			products = append(products, p)
		}
		if err := rows.Err(); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if products == nil {
			products = []Product{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"products": products,
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// --- Disputes handler --------------------------------------------------------

func handleListDisputes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		disputes, err := ListDisputes(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if disputes == nil {
			disputes = []Dispute{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"disputes": disputes,
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// --- Invoices handler --------------------------------------------------------

func handleListInvoices(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		invoices, err := ListInvoices(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if invoices == nil {
			invoices = []Invoice{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"invoices": invoices,
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// --- Webhook events handler --------------------------------------------------

func handleListWebhookEvents(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		events, err := ListWebhookEvents(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if events == nil {
			events = []WebhookEvent{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"webhook_events": events,
			"limit":          limit,
			"offset":         offset,
		})
	}
}

// --- Stats handler -----------------------------------------------------------

func handleStats(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := GetSyncStats(ctx, pool)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, stats)
	}
}

// --- Helpers -----------------------------------------------------------------

// parsePagination extracts limit and offset query parameters with defaults.
func parsePagination(r *http.Request) (int, int) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	return limit, offset
}
