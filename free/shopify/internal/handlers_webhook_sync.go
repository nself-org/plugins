package internal

import (
	"io"
	"log"
	"net/http"
)

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

