package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-post/internal"
)

func main() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.InternalSecret == "" {
		log.Fatal("POST_INTERNAL_SECRET is required")
	}
	if cfg.EncryptionKey == "" {
		log.Fatal("POST_ENCRYPTION_KEY is required")
	}

	// Parse encryption key: accept 64-char hex or 44-char base64 (32 bytes).
	encKey, err := internal.ParseEncryptionKey(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}

	ctx := context.Background()
	pool, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := internal.InitSchema(ctx, pool); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	h := &internal.Handlers{
		DB:  pool,
		Key: encKey,
	}

	r := chi.NewRouter()
	r.Use(internal.RequestIDMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(internal.PostMetricsMiddleware)

	// Health — unauthenticated
	r.Get("/health", h.HandleHealth)
	r.Handle("/metrics", internal.PostMetricsHandler())

	// Protected routes — require X-Internal-Token
	r.Group(func(r chi.Router) {
		r.Use(internalAuthMiddleware(cfg.InternalSecret))
		r.Post("/post/accounts", h.HandleCreateAccount)
		r.Get("/post/accounts", h.HandleListAccounts)
		r.Delete("/post/accounts/{id}", h.HandleDeleteAccount)
		r.Post("/post/publish", h.HandlePublish)
		r.Get("/post/queue", h.HandleListQueue)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
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
		log.Printf("nself-post listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("nself-post shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown: %v", err)
	}
	log.Println("nself-post stopped")
}

// internalAuthMiddleware rejects requests that do not present a valid
// X-Internal-Token header matching the configured secret.
func internalAuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Internal-Token")
			if token == "" {
				http.Error(w, "X-Internal-Token required", http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
				http.Error(w, "Invalid internal token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
