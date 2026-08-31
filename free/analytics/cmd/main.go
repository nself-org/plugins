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
	"github.com/nself-org/nself-analytics/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("analytics: failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		log.Fatalf("analytics: failed to initialize schema: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Optional API key auth middleware
	if cfg.APIKey != "" {
		r.Use(apiKeyMiddleware(cfg.APIKey))
	}

	// Health
	r.Get("/health", internal.HandleHealth)
	r.Get("/ready", internal.HandleReady(db))

	// Events
	r.Post("/api/v1/events", internal.HandleTrackEvent(db))
	r.Post("/api/v1/events/batch", internal.HandleTrackEventBatch(db, cfg.BatchSize))
	r.Get("/api/v1/events", internal.HandleListEvents(db))

	// Counters
	r.Post("/api/v1/counters/increment", internal.HandleIncrementCounter(db))
	r.Get("/api/v1/counters", internal.HandleListCounters(db))
	r.Get("/api/v1/counters/{key}", internal.HandleGetCounter(db))

	// Funnels
	r.Post("/api/v1/funnels", internal.HandleCreateFunnel(db))
	r.Get("/api/v1/funnels", internal.HandleListFunnels(db))
	r.Get("/api/v1/funnels/{id}/analyze", internal.HandleAnalyzeFunnel(db))

	// Quotas
	r.Post("/api/v1/quotas", internal.HandleCreateQuota(db))
	r.Get("/api/v1/quotas", internal.HandleListQuotas(db))

	// Violations
	r.Get("/api/v1/violations", internal.HandleListViolations(db))

	// Stats
	r.Get("/api/v1/stats", internal.HandleStats(db))
	r.Get("/api/v1/stats/timeseries", internal.HandleStatsTimeseries(db))

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("analytics: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("analytics: server error: %v", err)
		}
	}()

	<-quit
	log.Println("analytics: shutting down...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("analytics: forced shutdown: %v", err)
	}
	log.Println("analytics: stopped")
}

// apiKeyMiddleware rejects requests missing the correct X-API-Key header.
// Health endpoints bypass auth.
func apiKeyMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/health" || path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get("X-API-Key") != key {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
