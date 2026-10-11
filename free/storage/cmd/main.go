// Package main is the entrypoint for plugin-storage.
//
// Purpose: Start the plugin-storage HTTP server.
// Inputs: Environment variables (NSELF_DB_URL, STORAGE_PLUGIN_PORT, S3_* vars).
// Outputs: HTTP server on STORAGE_PLUGIN_PORT (default 9007).
// Constraints:
//   - S3_ENDPOINT is config-only; never user-overridable per request (SSRF guard).
//   - Requires NSELF_DB_URL for Postgres connection.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/plugin-storage/internal"
	"github.com/nself-org/plugin-storage/internal/s3store"
)

func main() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, cfg.DBUrl)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	internal.BootMigrations(ctx, db)
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("db ping error: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	var store *s3store.Store
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		store, err = s3store.New(s3store.Config{
			Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
			AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, UseSSL: cfg.UseSSL,
		})
		if err != nil {
			log.Fatalf("s3 config error: %v", err)
		}
		log.Printf("s3: endpoint %s region %s (path-style)", cfg.S3Endpoint, cfg.S3Region)
	} else if cfg.AccessKey != "" || cfg.SecretKey != "" {
		log.Fatalf("s3 config error: set both S3_ACCESS_KEY and S3_SECRET_KEY, or neither")
	} else {
		log.Printf("s3: not configured (S3_ACCESS_KEY/S3_SECRET_KEY unset); object routes answer 503")
	}

	srv := internal.NewServer(db, cfg, store)
	srv.Routes(r)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Minute, // large uploads stream through the handler
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("plugin-storage listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-done
	log.Println("shutting down plugin-storage")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
