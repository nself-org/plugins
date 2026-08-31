package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nself-org/nself-entitlements/internal"
)

// newRouter builds the chi router with all middleware and routes wired up.
// Extracted from main() so it can be exercised via httptest without binding
// a real port or connecting to a database (h.pool may be a pgxmock pool for
// route-shape tests; individual handler behavior is covered in the internal
// package).
func newRouter(h *internal.Handlers) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)
	r.Get("/v1/status", h.PluginStatus)

	// Plans
	r.Route("/api/v1/plans", func(r chi.Router) {
		r.Get("/", h.ListPlans)
		r.Post("/", h.CreatePlan)
		r.Get("/{id}", h.GetPlan)
		r.Put("/{id}", h.UpdatePlan)
		r.Delete("/{id}", h.DeletePlan)
	})

	// Subscriptions
	r.Route("/api/v1/subscriptions", func(r chi.Router) {
		r.Get("/", h.ListSubscriptions)
		r.Post("/", h.CreateSubscription)
		r.Get("/{id}", h.GetSubscription)
		r.Put("/{id}", h.UpdateSubscription)
		r.Post("/{id}/cancel", h.CancelSubscription)
		// P3 billing-backend lifecycle mutations
		r.Patch("/{id}/pause", h.PauseSubscription)
		r.Patch("/{id}/resume", h.ResumeSubscription)
		r.Get("/{id}/proration", h.ProrationPreview)
		r.Post("/{id}/upgrade", h.UpgradePlan)
	})

	// Grants (P3 billing backend — ad-hoc feature overrides)
	r.Route("/api/v1/grants", func(r chi.Router) {
		r.Get("/", h.ListGrants)
		r.Post("/", h.CreateGrant)
		r.Delete("/{id}", h.RevokeGrant)
	})

	// Batch entitlement check (P3 — up to 100 keys per request)
	r.Post("/api/v1/check/batch", h.BatchCheckEntitlement)

	// Features
	r.Route("/api/v1/features", func(r chi.Router) {
		r.Get("/", h.ListFeatures)
		r.Post("/", h.CreateFeature)
		r.Put("/{id}", h.UpdateFeature)
		r.Delete("/{id}", h.DeleteFeature)
	})

	// Quotas
	r.Route("/api/v1/quotas", func(r chi.Router) {
		r.Get("/", h.ListQuotas)
		r.Post("/", h.CreateQuota)
		r.Put("/{id}", h.UpdateQuota)
		r.Delete("/{id}", h.DeleteQuota)
	})

	// Usage
	r.Post("/api/v1/usage/track", h.TrackUsage)
	r.Get("/api/v1/usage", h.GetUsage)

	// Entitlement check
	r.Post("/api/v1/check", h.CheckEntitlement)

	// Addons
	r.Route("/api/v1/addons", func(r chi.Router) {
		r.Get("/", h.ListAddons)
		r.Post("/", h.CreateAddon)
	})

	return r
}

func main() {
	cfg := internal.LoadConfig()

	pool, err := internal.NewDB(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	h := internal.NewHandlers(pool)
	r := newRouter(h)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("entitlements plugin listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
