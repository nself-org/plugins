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
	"github.com/nself-org/nself-ddns/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("ddns: failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		log.Fatalf("ddns: failed to initialize schema: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	if cfg.APIKey != "" {
		r.Use(apiKeyMiddleware(cfg.APIKey))
	}

	// Health
	r.Get("/health", internal.HandleHealth)
	r.Get("/ready", internal.HandleReady(db))

	// IP detection
	r.Get("/api/v1/ip", internal.HandleGetIP)

	// DDNS configurations
	r.Get("/api/v1/configs", internal.HandleListConfigs(db))
	r.Post("/api/v1/configs", internal.HandleCreateConfig(db))
	r.Get("/api/v1/configs/{id}", internal.HandleGetConfig(db))
	r.Put("/api/v1/configs/{id}", internal.HandleUpdateConfig(db))
	r.Delete("/api/v1/configs/{id}", internal.HandleDeleteConfig(db))
	r.Post("/api/v1/configs/{id}/update", internal.HandleManualUpdate(db))

	// Bulk auto-update (cron target)
	r.Post("/api/v1/update", internal.HandleAutoUpdate(db))

	// Update log
	r.Get("/api/v1/update-log", internal.HandleListUpdateLog(db))
	r.Get("/api/v1/update-log/{config_id}", internal.HandleListUpdateLogForConfig(db))

	// Stats
	r.Get("/api/v1/stats", internal.HandleStats(db))

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("ddns: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ddns: server error: %v", err)
		}
	}()

	<-quit
	log.Println("ddns: shutting down...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("ddns: forced shutdown: %v", err)
	}
	log.Println("ddns: stopped")
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
