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
	"github.com/nself-org/nself-object-storage/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := internal.NewPool(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	h := internal.NewHandler(pool, cfg)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health
	r.Get("/health", h.HealthCheck)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Buckets
		r.Get("/buckets", h.ListBuckets)
		r.Post("/buckets", h.CreateBucket)
		r.Get("/buckets/{id}", h.GetBucket)
		r.Put("/buckets/{id}", h.UpdateBucket)
		r.Delete("/buckets/{id}", h.DeleteBucket)

		// Objects (key may contain slashes — use wildcard segment)
		r.Get("/buckets/{id}/objects", h.ListObjects)
		r.Post("/buckets/{id}/objects", h.CreateObject)
		r.Get("/buckets/{id}/objects/{key}", h.GetObject)
		r.Delete("/buckets/{id}/objects/{key}", h.DeleteObject)

		// Upload Sessions
		r.Post("/upload-sessions", h.CreateUploadSession)
		r.Put("/upload-sessions/{id}", h.UpdateUploadSession)
		r.Delete("/upload-sessions/{id}", h.DeleteUploadSession)

		// Access Logs
		r.Get("/access-logs", h.ListAccessLogs)

		// Stats
		r.Get("/stats", h.GetStats)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("nself-object-storage listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("stopped")
}
