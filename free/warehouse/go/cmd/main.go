// nself-warehouse: Postgres → ClickHouse / BigQuery / Snowflake export plugin (CS_6).
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nself-org/nself-warehouse/api"
	"github.com/nself-org/nself-warehouse/pipeline"
)

func main() {
	cfg := pipeline.LoadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.Driver == "" {
		log.Fatal("WAREHOUSE_DRIVER is required (clickhouse|bigquery|snowflake)")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Run migrations
	if err := pipeline.Migrate(ctx, cfg.DatabaseURL); err != nil {
		log.Fatalf("migration: %v", err)
	}

	// Build the export pipeline (consumer + batcher + backfill)
	p, err := pipeline.New(ctx, cfg)
	if err != nil {
		log.Fatalf("pipeline init: %v", err)
	}

	// Start streaming in the background
	go func() {
		if err := p.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("pipeline error: %v", err)
		}
	}()

	// HTTP control plane
	mux := api.NewMux(p)
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("warehouse listening", "addr", addr, "driver", cfg.Driver)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	slog.Info("warehouse shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	p.Close()
}
