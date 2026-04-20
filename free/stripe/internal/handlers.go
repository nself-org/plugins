package internal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server holds the HTTP server dependencies.
type Server struct {
	Config   *Config
	DB       *DB
	Client   *StripeClient
	Accounts []StripeAccountConfig
	StartAt  time.Time
}

// NewRouter creates the chi router with all routes.
func (s *Server) NewRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(corsMiddleware)

	// Health check endpoints (no auth)
	r.Get("/health", s.handleHealth)
	r.Get("/ready", s.handleReady)
	r.Get("/live", s.handleLive)
	r.Get("/status", s.handleStatus)

	// Webhook endpoint (Stripe signature verified internally)
	r.Post("/webhooks/stripe", s.handleWebhook)

	// Sync endpoints
	r.Post("/sync", s.handleSync)
	r.Post("/v1/sync", s.handleSync)
	r.Post("/v1/reconcile", s.handleReconcile)

	// API endpoints with source_account_id scoping
	r.Route("/api", func(r chi.Router) {
		r.Get("/customers", s.handleListCustomers)
		r.Get("/customers/{id}", s.handleGetCustomer)
		r.Get("/products", s.handleListProducts)
		r.Get("/products/{id}", s.handleGetProduct)
		r.Get("/prices", s.handleListPrices)
		r.Get("/prices/{id}", s.handleGetPrice)
		r.Get("/subscriptions", s.handleListSubscriptions)
		r.Get("/subscriptions/{id}", s.handleGetSubscription)
		r.Get("/invoices", s.handleListInvoices)
		r.Get("/invoices/{id}", s.handleGetInvoice)
		r.Get("/payment-intents", s.handleListPaymentIntents)
		r.Get("/payment-intents/{id}", s.handleGetPaymentIntent)
		r.Get("/charges", s.handleListCharges)
		r.Get("/charges/{id}", s.handleGetCharge)
		r.Get("/refunds", s.handleListRefunds)
		r.Get("/refunds/{id}", s.handleGetRefund)
		r.Get("/coupons", s.handleListCoupons)
		r.Get("/coupons/{id}", s.handleGetCoupon)
		r.Get("/balance-transactions", s.handleListBalanceTransactions)
		r.Get("/balance-transactions/{id}", s.handleGetBalanceTransaction)
		r.Get("/payouts", s.handleListPayouts)
		r.Get("/payouts/{id}", s.handleGetPayout)
		r.Get("/disputes", s.handleListDisputes)
		r.Get("/disputes/{id}", s.handleGetDispute)
		r.Get("/events", s.handleListEvents)
		r.Get("/checkout-sessions", s.handleListCheckoutSessions)
		r.Get("/checkout-sessions/{id}", s.handleGetCheckoutSession)
		r.Get("/stats", s.handleStats)
	})

	return r
}

// scopedDB returns a DB scoped to the source_account_id from the request header.
func (s *Server) scopedDB(r *http.Request) *DB {
	accountID := r.Header.Get("X-Source-Account-ID")
	if accountID == "" {
		accountID = r.URL.Query().Get("source_account_id")
	}
	if accountID == "" {
		return s.DB
	}
	return s.DB.ForSourceAccount(accountID)
}

// ============================================================================
// Health Checks
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"plugin":    "stripe",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	err := s.DB.Pool.Ping(r.Context())
	if err != nil {
		log.Printf("[stripe:health] Readiness check failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ready":     false,
			"plugin":    "stripe",
			"error":     "Database unavailable",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":     true,
		"plugin":    "stripe",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	stats, err := db.GetStats(r.Context())
	if err != nil {
		log.Printf("[stripe:health] Live check stats failed: %v", err)
		stats = &SyncStats{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alive":   true,
		"plugin":  "stripe",
		"version": "1.0.0",
		"uptime":  time.Since(s.StartAt).Seconds(),
		"stats": map[string]interface{}{
			"customers":     stats.Customers,
			"subscriptions": stats.Subscriptions,
			"lastSync":      stats.LastSyncedAt,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	stats, err := db.GetStats(r.Context())
	if err != nil {
		log.Printf("[stripe:health] Status stats failed: %v", err)
		stats = &SyncStats{}
	}

	accountIDs := make([]string, len(s.Accounts))
	for i, acc := range s.Accounts {
		accountIDs[i] = acc.ID
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"plugin":    "stripe",
		"version":   "1.0.0",
		"status":    "running",
		"accounts":  accountIDs,
		"stats":     stats,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// ============================================================================
// Webhook
// ============================================================================

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("Stripe-Signature")
	if signature == "" {
		log.Println("[stripe:webhooks] Missing Stripe signature header")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing signature"})
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[stripe:webhooks] Failed to read body: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read body"})
		return
	}
	defer r.Body.Close()

	// Find matching account by signature
	matchIdx := FindMatchingAccount(rawBody, signature, s.Accounts)
	if matchIdx < 0 {
		log.Println("[stripe:webhooks] Invalid Stripe signature for all configured accounts")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid signature"})
		return
	}

	matchedAccount := s.Accounts[matchIdx]

	if matchedAccount.WebhookSecret == "" {
		log.Println("[stripe:webhooks] Webhook secret not configured for matched account")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Webhook secret not configured"})
		return
	}

	// Parse the event
	var event StripeEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		log.Printf("[stripe:webhooks] Failed to parse event: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid event payload"})
		return
	}

	// Process the event with the matched account's scoped DB
	scopedDB := s.DB.ForSourceAccount(matchedAccount.ID)

	// Idempotency check (S76-T05): have we already processed this Stripe event ID?
	// Mirrors the pattern in ping_api/src/routes/webhooks.ts (line 156) so both
	// canonical paths guarantee at-most-once processing. The stripe_events table
	// is created by the plugin migration (schema/tables.sql) with IF NOT EXISTS.
	if event.ID != "" {
		var exists bool
		_ = scopedDB.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM stripe_events WHERE stripe_event_id = $1 AND source_account_id = $2)`,
			event.ID, scopedDB.SourceAccountID,
		).Scan(&exists)
		if exists {
			log.Printf("[stripe:webhooks] Event already processed (idempotent): %s", event.ID)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"received":   true,
				"idempotent": true,
			})
			return
		}
	}

	handler := NewWebhookHandler(scopedDB)

	if err := handler.HandleEvent(r.Context(), &event); err != nil {
		log.Printf("[stripe:webhooks] Processing failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Processing failed"})
		return
	}

	// Record processed event for idempotency. Non-fatal if stripe_events table
	// doesn't exist yet (e.g., plugin just installed, migration pending).
	if event.ID != "" {
		_, err := scopedDB.Pool.Exec(r.Context(),
			`INSERT INTO stripe_events (stripe_event_id, source_account_id, event_type, processed_at)
			 VALUES ($1, $2, $3, NOW())
			 ON CONFLICT (stripe_event_id) DO NOTHING`,
			event.ID, scopedDB.SourceAccountID, event.Type,
		)
		if err != nil {
			log.Printf("[stripe:webhooks] Warning: could not record event %s for idempotency: %v", event.ID, err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"received": true,
		"account":  matchedAccount.ID,
	})
}

// ============================================================================
// Sync
// ============================================================================

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if s.Client == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Stripe client not configured"})
		return
	}

	results := SyncAll(r.Context(), s.DB, s.Client, s.Accounts)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if s.Client == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Stripe client not configured"})
		return
	}

	results := Reconcile(r.Context(), s.DB, s.Client, s.Accounts)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}

// ============================================================================
// API Endpoints
// ============================================================================

func (s *Server) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCustomers(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCustomers(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCustomer(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_customers")
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListProducts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountProducts(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_products")
}

func (s *Server) handleListPrices(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListPrices(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountPrices(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetPrice(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_prices")
}

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	data, err := db.ListSubscriptions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountSubscriptions(r.Context(), status)
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_subscriptions")
}

func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	data, err := db.ListInvoices(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountInvoices(r.Context(), status)
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_invoices")
}

func (s *Server) handleListPaymentIntents(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListPaymentIntents(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountPaymentIntents(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_payment_intents")
}

func (s *Server) handleListCharges(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCharges(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCharges(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCharge(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_charges")
}

func (s *Server) handleListRefunds(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListRefunds(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountRefunds(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetRefund(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_refunds")
}

func (s *Server) handleListCoupons(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCoupons(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCoupons(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCoupon(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_coupons")
}

func (s *Server) handleListBalanceTransactions(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListBalanceTransactions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountBalanceTransactions(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetBalanceTransaction(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_balance_transactions")
}

func (s *Server) handleListPayouts(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListPayouts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountPayouts(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetPayout(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_payouts")
}

func (s *Server) handleListDisputes(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListDisputes(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountDisputes(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetDispute(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_disputes")
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	eventType := r.URL.Query().Get("type")
	data, err := db.ListEvents(r.Context(), eventType, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":   data,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleListCheckoutSessions(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCheckoutSessions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCheckoutSessions(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCheckoutSession(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_checkout_sessions")
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	stats, err := db.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ============================================================================
// Helpers
// ============================================================================

func (s *Server) handleGetByID(w http.ResponseWriter, r *http.Request, table string) {
	db := s.scopedDB(r)
	id := chi.URLParam(r, "id")
	data, err := db.getRow(r.Context(), table, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if data == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func parsePagination(r *http.Request) (int, int) {
	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}
	return limit, offset
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// stripeAllowedOrigins returns the per-request origin-allowlist populated
// from STRIPE_ALLOWED_ORIGINS (csv). An empty env var means no cross-origin
// requests are allowed. Wildcard ("*") is never accepted since this plugin
// emits Access-Control-Allow-Credentials: true.
func stripeAllowedOrigins() map[string]bool {
	set := make(map[string]bool)
	for _, o := range strings.Split(os.Getenv("STRIPE_ALLOWED_ORIGINS"), ",") {
		o = strings.TrimSpace(strings.ToLower(o))
		if o == "" || o == "*" {
			continue
		}
		set[o] = true
	}
	return set
}

// corsMiddleware enforces explicit per-origin CORS. If the request Origin
// is not in the allowlist, CORS headers are NOT emitted and the browser
// blocks the response. Credentials are only permitted alongside an echoed,
// allowlisted origin — never with a wildcard.
//
// Webhook endpoints (invoked server-to-server by Stripe) must NOT be
// mounted behind this middleware; Stripe calls them directly and no
// Origin header is involved. See NewRouter for route grouping.
func corsMiddleware(next http.Handler) http.Handler {
	allowed := stripeAllowedOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Origin")
		origin := r.Header.Get("Origin")
		originAllowed := origin != "" && allowed[strings.ToLower(origin)]
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Source-Account-ID, Stripe-Signature")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == "OPTIONS" {
			if !originAllowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
