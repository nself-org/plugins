// Command payments is the nSelf payments plugin service.
// It listens on PAYMENTS_PORT (default 3086) and provides a unified
// payment API over Stripe, Lemon Squeezy, and Paddle.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nself-org/nself-payments/internal/config"
	"github.com/nself-org/nself-payments/internal/db"
	"github.com/nself-org/nself-payments/internal/dunning"
	"github.com/nself-org/nself-payments/internal/metrics"
	"github.com/nself-org/nself-payments/internal/provider"
	"github.com/nself-org/nself-payments/internal/provider/lemonsqueezy"
	"github.com/nself-org/nself-payments/internal/provider/paddle"
	stripeprovider "github.com/nself-org/nself-payments/internal/provider/stripe"
	"github.com/nself-org/nself-payments/internal/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 60*time.Second)
	if err := db.Migrate(migrateCtx, pool); err != nil {
		migrateCancel()
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	migrateCancel()

	// Build provider registry.
	providers := make(map[string]provider.Provider)

	stripeAdapter := stripeprovider.New(cfg.StripeSecretKey)
	providers["stripe"] = stripeAdapter

	lsAdapter := lemonsqueezy.New(cfg.LemonSqueezyAPIKey, cfg.LemonSqueezyStoreID)
	providers["lemonsqueezy"] = lsAdapter

	if cfg.PaddleAPIKey != "" {
		paddleAdapter, err := paddle.New(cfg.PaddleAPIKey, cfg.PaddlePublicKey)
		if err != nil {
			slog.Error("paddle adapter init failed", "error", err)
			os.Exit(1)
		}
		providers["paddle"] = paddleAdapter
	}

	defaultProvider, ok := providers[cfg.DefaultProvider]
	if !ok {
		slog.Error("default provider not available", "provider", cfg.DefaultProvider)
		os.Exit(1)
	}

	// Metrics registry.
	m := metrics.New()

	// HTTP router.
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	h := server.New(pool, providers, defaultProvider, cfg, m)
	h.Register(r)

	// Prometheus metrics endpoint.
	r.Handle("/metrics", promhttp.Handler())

	// Health endpoint.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","plugin":"payments"}`)
	})

	// Start dunning engine.
	dunningEngine := dunning.New(pool, providers, cfg, m)
	stopDunning := dunningEngine.Start()
	defer stopDunning()

	srv := &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("payments plugin starting", "addr", srv.Addr, "provider", cfg.DefaultProvider)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down payments plugin")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
