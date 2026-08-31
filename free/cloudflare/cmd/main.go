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
	"github.com/nself-org/nself-cloudflare/internal"
)

// newRouter builds the chi router with all cloudflare routes wired to db.
// Extracted from main() so route wiring is unit-testable via httptest without
// starting a real listener or touching a live database.
func newRouter(db *internal.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Health
	r.Get("/health", internal.HandleHealth)
	r.Get("/ready", internal.HandleReady(db))

	// Zones
	r.Get("/api/v1/zones", internal.HandleListZones(db))
	r.Post("/api/v1/zones", internal.HandleCreateZone(db))
	r.Get("/api/v1/zones/{id}", internal.HandleGetZone(db))
	r.Delete("/api/v1/zones/{id}", internal.HandleDeleteZone(db))

	// DNS Records
	r.Get("/api/v1/dns-records", internal.HandleListDNSRecords(db))
	r.Post("/api/v1/dns-records", internal.HandleCreateDNSRecord(db))
	r.Put("/api/v1/dns-records/{id}", internal.HandleUpdateDNSRecord(db))
	r.Delete("/api/v1/dns-records/{id}", internal.HandleDeleteDNSRecord(db))

	// Cache
	r.Post("/api/v1/cache/purge", internal.HandlePurgeCache(db))
	r.Get("/api/v1/cache/purge-log", internal.HandleListCachePurgeLog(db))

	// R2 Buckets
	r.Get("/api/v1/r2/buckets", internal.HandleListR2Buckets(db))
	r.Post("/api/v1/r2/buckets", internal.HandleCreateR2Bucket(db))
	r.Delete("/api/v1/r2/buckets/{id}", internal.HandleDeleteR2Bucket(db))

	// Analytics
	r.Get("/api/v1/analytics", internal.HandleGetAnalytics(db))

	// Stats
	r.Get("/api/v1/stats", internal.HandleStats(db))

	return r
}

// serverAddr formats the listen address from config. Extracted so the
// host:port formatting rule is unit-testable in isolation.
func serverAddr(cfg internal.Config) string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// newServer builds the http.Server for addr wrapping r. Extracted for
// unit-testability of the timeout/address wiring without binding a socket.
func newServer(addr string, r *chi.Mux) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func main() {
	cfg := internal.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("cloudflare: failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		log.Fatalf("cloudflare: failed to initialize schema: %v", err)
	}

	r := newRouter(db)

	addr := serverAddr(cfg)
	srv := newServer(addr, r)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("cloudflare: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("cloudflare: server error: %v", err)
		}
	}()

	<-quit
	log.Println("cloudflare: shutting down...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("cloudflare: forced shutdown: %v", err)
	}
	log.Println("cloudflare: stopped")
}
