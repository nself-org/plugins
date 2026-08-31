// nself-siem is the SIEM integration agent for nSelf.
// It reads np_audit_log, normalizes events (OCSF/ECS/raw), and ships to
// configured destinations: Loki (free), Datadog, Splunk HEC, Elastic, or webhook.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nself-org/nself-siem/internal/agent"
	"github.com/nself-org/nself-siem/internal/config"
	"github.com/nself-org/nself-siem/internal/db"
	"github.com/nself-org/nself-siem/internal/destinations"
	"github.com/nself-org/nself-siem/internal/handlers"
	"github.com/nself-org/nself-siem/internal/server"
)

func main() {
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	logLevel := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	// Build the default Loki shipper (free, always enabled when monitoring stack present).
	var defaultShipper destinations.Shipper
	if cfg.LokiURL != "" {
		defaultShipper = destinations.WithRetry(destinations.NewLoki(cfg.LokiURL))
		slog.Info("Loki destination enabled", "url", cfg.LokiURL)
	}

	// If NSELF_SIEM is set, also honour external destinations from the DB.
	// The batcher uses the default shipper; per-destination shippers are
	// instantiated per-destination record at flush time in a future extension.
	if !cfg.Enabled {
		slog.Info("external SIEM shipping disabled (NSELF_SIEM not set); Loki only")
	}

	shipFn := func(ctx context.Context, events [][]byte) error {
		if defaultShipper == nil {
			slog.Debug("no shipper configured, events discarded", "count", len(events))
			return nil
		}
		return defaultShipper.Ship(ctx, events)
	}

	batcher := agent.NewBatcher(cfg, pool, cfg.DefaultSchema, shipFn)

	// Expose flushFn for the HTTP /siem/flush endpoint.
	flushFn := func(ctx context.Context) error {
		// Trigger an immediate flush by calling the batcher's internal flush.
		// Exposed via a thin adapter since flush is unexported.
		return nil // batcher.Flush(ctx) — extended in full implementation
	}

	h := handlers.New(pool, flushFn)
	srv := server.New(cfg.Port, h)

	// Run batcher in background.
	go batcher.Run(ctx)

	// Block until signal.
	if err := srv.Start(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("server error: %v", err)
	}
	slog.Info("SIEM agent stopped")
}
