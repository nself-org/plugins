package internal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

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

// -------------------------------------------------------------------------
// Health endpoints
// -------------------------------------------------------------------------

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"plugin":    "shopify",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Pool.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"ready":     false,
				"plugin":    "shopify",
				"error":     "Database unavailable",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ready":     true,
			"plugin":    "shopify",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleLive(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, _ := db.GetStats(r.Context())
		shop, _ := db.GetShop(r.Context())

		var shopInfo interface{}
		if shop != nil {
			shopInfo = map[string]interface{}{
				"name":   shop.Name,
				"domain": shop.Domain,
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"alive":     true,
			"plugin":    "shopify",
			"version":   "1.0.0",
			"shop":      shopInfo,
			"stats":     stats,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// -------------------------------------------------------------------------
// Status / Stats
// -------------------------------------------------------------------------

func handleStatus(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetStats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		shop, _ := db.GetShop(r.Context())

		var shopInfo interface{}
		if shop != nil {
			shopInfo = map[string]interface{}{
				"name":   shop.Name,
				"domain": shop.Domain,
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"shop":  shopInfo,
			"stats": stats,
		})
	}
}

func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetStats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}

// -------------------------------------------------------------------------
// Webhook endpoint
// -------------------------------------------------------------------------

func handleWebhook(proc *WebhookProcessor, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := r.Header.Get("X-Shopify-Topic")
		shopDomain := r.Header.Get("X-Shopify-Shop-Domain")
		hmacHeader := r.Header.Get("X-Shopify-Hmac-Sha256")
		webhookID := r.Header.Get("X-Shopify-Webhook-Id")

		if topic == "" {
			writeError(w, http.StatusBadRequest, "Missing X-Shopify-Topic header")
			return
		}
		if shopDomain == "" {
			writeError(w, http.StatusBadRequest, "Missing X-Shopify-Shop-Domain header")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Failed to read body")
			return
		}

		// Verify HMAC signature if secret is configured
		if webhookSecret != "" && hmacHeader != "" {
			if !VerifyHMAC(body, webhookSecret, hmacHeader) {
				log.Printf("[shopify:webhook] HMAC verification failed: topic=%s shop=%s", topic, shopDomain)
				writeError(w, http.StatusUnauthorized, "Invalid signature")
				return
			}
		}

		if err := proc.ProcessEvent(r.Context(), webhookID, topic, shopDomain, body); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"received": true})
	}
}

// -------------------------------------------------------------------------
// Sync endpoint
// -------------------------------------------------------------------------

func handleSync(db *DB, client *ShopifyAPIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		result := &SyncResult{AccountID: db.SourceAccountID}

		// Sync shop
		shop, err := client.GetShop(ctx)
		if err != nil {
			result.Error = err.Error()
			writeJSON(w, http.StatusInternalServerError, result)
			return
		}
		if err := db.UpsertShop(ctx, shop); err != nil {
			log.Printf("[shopify:sync] Shop upsert failed: %v", err)
		}

		// Sync products + variants
		products, variants, err := client.ListAllProducts(ctx)
		if err != nil {
			log.Printf("[shopify:sync] Products fetch failed: %v", err)
		} else {
			for _, p := range products {
				productUUID, err := db.UpsertProduct(ctx, &p)
				if err != nil {
					log.Printf("[shopify:sync] Product upsert failed: %v", err)
					continue
				}
				result.Products++
				for _, v := range variants {
					if v.ShopifyID != 0 {
						v.ProductID = productUUID
						if err := db.UpsertVariant(ctx, &v); err != nil {
							log.Printf("[shopify:sync] Variant upsert failed: %v", err)
							continue
						}
						result.Variants++
					}
				}
			}
		}

		// Sync collections
		collections, err := client.ListAllCollections(ctx)
		if err != nil {
			log.Printf("[shopify:sync] Collections fetch failed: %v", err)
		} else {
			for _, c := range collections {
				if err := db.UpsertCollection(ctx, &c); err != nil {
					log.Printf("[shopify:sync] Collection upsert failed: %v", err)
					continue
				}
				result.Collections++
			}
		}

		// Sync customers
		customers, err := client.ListAllCustomers(ctx)
		if err != nil {
			log.Printf("[shopify:sync] Customers fetch failed: %v", err)
		} else {
			for _, c := range customers {
				if err := db.UpsertCustomer(ctx, &c); err != nil {
					log.Printf("[shopify:sync] Customer upsert failed: %v", err)
					continue
				}
				result.Customers++
			}
		}

		// Sync orders + line items
		orders, orderItems, err := client.ListAllOrders(ctx)
		if err != nil {
			log.Printf("[shopify:sync] Orders fetch failed: %v", err)
		} else {
			for _, o := range orders {
				orderUUID, err := db.UpsertOrder(ctx, &o)
				if err != nil {
					log.Printf("[shopify:sync] Order upsert failed: %v", err)
					continue
				}
				result.Orders++
				for _, item := range orderItems {
					item.OrderID = orderUUID
					if err := db.UpsertOrderItem(ctx, &item); err != nil {
						log.Printf("[shopify:sync] OrderItem upsert failed: %v", err)
						continue
					}
					result.OrderItems++
				}
			}
		}

		// Sync inventory
		inventory, err := client.ListInventoryLevels(ctx)
		if err != nil {
			log.Printf("[shopify:sync] Inventory fetch failed: %v", err)
		} else {
			for _, inv := range inventory {
				if err := db.UpsertInventoryLevel(ctx, inv.InventoryItemID, inv.LocationID, inv.Available); err != nil {
					log.Printf("[shopify:sync] Inventory upsert failed: %v", err)
					continue
				}
				result.Inventory++
			}
		}

		log.Printf("[shopify:sync] Complete: %d products, %d variants, %d collections, %d customers, %d orders, %d inventory",
			result.Products, result.Variants, result.Collections, result.Customers, result.Orders, result.Inventory)

		writeJSON(w, http.StatusOK, result)
	}
}

// -------------------------------------------------------------------------
// List / Get handlers
// -------------------------------------------------------------------------

func handleListShops(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shop, err := db.GetShop(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var data []interface{}
		if shop != nil {
			data = append(data, shop)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  data,
			"total": len(data),
		})
	}
}

func handleListProducts(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		products, err := db.ListProducts(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountProducts(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": products, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleGetProduct(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		product, err := db.GetProduct(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if product == nil {
			writeError(w, http.StatusNotFound, "Product not found")
			return
		}
		variants, _ := db.GetProductVariants(r.Context(), product.ID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"product": product, "variants": variants,
		})
	}
}

func handleListVariants(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		variants, err := db.ListVariants(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountVariants(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": variants, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleListCollections(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		collections, err := db.ListCollections(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountCollections(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": collections, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleListCustomers(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		customers, err := db.ListCustomers(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountCustomers(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": customers, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleGetCustomer(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		customer, err := db.GetCustomer(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if customer == nil {
			writeError(w, http.StatusNotFound, "Customer not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"customer": customer})
	}
}

func handleListOrders(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		status := r.URL.Query().Get("status")
		orders, err := db.ListOrders(r.Context(), status, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountOrders(r.Context(), status)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": orders, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleGetOrder(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		order, err := db.GetOrder(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if order == nil {
			writeError(w, http.StatusNotFound, "Order not found")
			return
		}
		items, _ := db.GetOrderItems(r.Context(), order.ID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"order": order, "items": items,
		})
	}
}

func handleListInventory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		inventory, err := db.ListInventory(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountInventory(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": inventory, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleListEvents(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		limit := queryInt(r, "limit", 50)
		events, err := db.ListWebhookEvents(r.Context(), topic, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": events})
	}
}

// -------------------------------------------------------------------------
// HTTP helpers
// -------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[shopify:http] JSON encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{"error": msg})
}

func pagination(r *http.Request) (int, int) {
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	if limit < 1 {
		limit = 100
	}
	if limit > 250 {
		limit = 250
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func queryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
