package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-jobs/internal"
)

func main() {
	port := envInt("JOBS_PORT", 3105)
	databaseURL := envStr("DATABASE_URL", "")
	pollInterval := envInt("JOBS_POLL_INTERVAL_MS", 1000)
	maxAttempts := envInt("JOBS_RETRY_ATTEMPTS", 3)

	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	log.Println("database connected")

	db := internal.NewDB(pool)

	if err := db.EnsureTables(ctx); err != nil {
		log.Fatalf("failed to create tables: %v", err)
	}
	log.Println("tables verified")

	h := internal.NewHandlers(db)

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/jobs", h.CreateJob)
		r.Get("/jobs", h.ListJobs)
		r.Get("/jobs/{id}", h.GetJob)
		r.Delete("/jobs/{id}", h.DeleteJob)
		r.Post("/jobs/{id}/retry", h.RetryJob)
		r.Get("/queues", h.ListQueues)
		// S18 DLQ surface — operators list dead-lettered jobs and revive them
		// after a downstream outage is resolved.
		r.Get("/dlq", h.ListDLQ)
		r.Post("/dlq/{id}/revive", h.ReviveDLQ)
	})

	// Start background worker
	w := internal.NewWorker(db, time.Duration(pollInterval)*time.Millisecond, maxAttempts)
	go w.Run(ctx)
	log.Println("worker started")

	addr := net.JoinHostPort("", strconv.Itoa(port))
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-quit:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	log.Println("shutdown complete")
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return n
	}
	return fallback
}

