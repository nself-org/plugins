package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	server "github.com/nself-org/plugins/free/shared/go/server"
)

// dbPool holds the optional pgxpool used by the readiness check.
// Stored via atomic.Pointer so handleReady can read it without locking.
// nil pool means readiness reports "ready" (bootstrap mode — DB not yet
// wired in S6.T07; readiness fails only when a pool is configured AND ping fails).
var dbPool atomic.Pointer[pgxpool.Pool]

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getEnv("PORT", "3845")

	// Optional DB pool for readiness check. When DATABASE_URL is set we
	// open a pool and use it for /ready; otherwise /ready reports ready in
	// bootstrap mode (S6.T07 stub state — full DB wiring lands with the
	// signup/login handlers).
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err := pgxpool.New(ctx, dsn)
		cancel()
		if err != nil {
			log.Printf("nself-cloud: pgxpool.New failed (continuing without DB): %v", err)
		} else {
			dbPool.Store(pool)
			defer pool.Close()
		}
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health / readiness — no auth
	r.Get("/health", handleHealth)
	r.Get("/ready", handleReady)
	r.Get("/metrics", handleMetricsStub)

	// Cloud API — full implementation in subsequent tickets (S6.T07+)
	r.Route("/api/cloud", func(r chi.Router) {
		// Public
		r.Post("/signup", handleNotImplemented)
		r.Post("/login", handleNotImplemented)
		r.Post("/waitlist", handleNotImplemented)

		// Authenticated
		r.Get("/me", handleNotImplemented)
		r.Route("/instances", func(r chi.Router) {
			r.Get("/", handleNotImplemented)
			r.Post("/", handleNotImplemented)
			r.Get("/{id}", handleNotImplemented)
			r.Delete("/{id}", handleNotImplemented)
			r.Post("/{id}/domain", handleNotImplemented)
			r.Get("/{id}/domain/verify", handleNotImplemented)
			r.Get("/{id}/logs", handleNotImplemented)
		})
		r.Post("/invitations", handleNotImplemented)
		r.Get("/invitations/accept", handleNotImplemented)
	})

	// Billing webhooks — HMAC verified, raw body required (full impl in S6.T09)
	// SEC-CRITICAL: HMAC verification implemented in S6.T09
	r.Post("/webhooks/stripe", handleWebhookStub)
	r.Post("/webhooks/lemonsqueezy", handleWebhookStub)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		log.Printf("nself-cloud on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	server.GracefulShutdown(srv, 30*time.Second)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pool := dbPool.Load()
	if pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
				"status": "not_ready",
				"reason": "db_ping_failed",
				"error":  err.Error(),
			})
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"}) //nolint:errcheck
}

func handleMetricsStub(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "# HELP nself_cloud_up nself-cloud plugin liveness\n# TYPE nself_cloud_up gauge\nnself_cloud_up 1")
}

// handleWebhookStub is a stub for billing webhook processing.
// Full HMAC verification + event routing implemented in S6.T09.
// SEC-CRITICAL: never process events without HMAC verification.
func handleWebhookStub(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"error":  "not implemented",
		"detail": "webhook processing implemented in S6.T09",
	})
}

func handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"}) //nolint:errcheck
}
