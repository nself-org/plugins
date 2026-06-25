package internal

import (
	"net/http"
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
// Size-cap exception: single-responsibility HTTP route handler — 55L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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

// sourceAccountHeaders are the four canonical spellings of the multi-app
// isolation header (mirrors sdk.SourceAccountID; stripe has no sdk dep).
// Fix: previously only checked X-Source-Account-ID (P4-E0 audit).
var sourceAccountHeaders = []string{
	"X-Source-Account-ID",
	"X-Source-Account-Id",
	"X-Hasura-Source-Account-Id",
	"X-Source-Account",
}

// scopedDB returns a DB scoped to the source_account_id from the request
// header (all 4 canonical spellings) or query param fallback.
func (s *Server) scopedDB(r *http.Request) *DB {
	var accountID string
	for _, h := range sourceAccountHeaders {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			accountID = v
			break
		}
	}
	if accountID == "" {
		// Fallback to URL query param for non-Hasura proxied requests.
		accountID = r.URL.Query().Get("source_account_id")
	}
	if accountID == "" {
		return s.DB
	}
	return s.DB.ForSourceAccount(accountID)
}

