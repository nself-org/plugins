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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-byok/handlers"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	port := os.Getenv("BYOK_PLUGIN_PORT")
	if port == "" {
		port = "3741"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}

	if err := initSchema(ctx, db); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	handlers.RegisterRoutes(r, db)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("byok plugin listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down byok plugin...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("byok plugin stopped")
}

// initSchema runs the BYOK migration SQL inline if the tables do not exist.
// In production nSelf runs migrations via the CLI migration runner; this
// ensures the plugin is self-bootstrapping in dev and test environments.
func initSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS np_kms_configs (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        UUID,
  provider         TEXT NOT NULL CHECK (provider IN ('aws','gcp','vault')),
  key_ref          TEXT NOT NULL,
  region           TEXT,
  endpoint_url     TEXT,
  credentials_ref  TEXT,
  enabled          BOOLEAN NOT NULL DEFAULT TRUE,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_verified_at TIMESTAMPTZ,
  UNIQUE (tenant_id)
);

CREATE TABLE IF NOT EXISTS np_encrypted_values (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id      UUID,
  record_group   TEXT NOT NULL,
  field_name     TEXT NOT NULL,
  encrypted_data BYTEA NOT NULL,
  encrypted_dek  BYTEA NOT NULL,
  kms_key_ref    TEXT NOT NULL,
  iv             BYTEA NOT NULL,
  algorithm      TEXT NOT NULL DEFAULT 'AES-256-GCM',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_encryption_key_events (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID,
  event_type   TEXT NOT NULL,
  key_ref      TEXT NOT NULL,
  triggered_by UUID,
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  details      JSONB NOT NULL DEFAULT '{}'
);
`)
	return err
}
